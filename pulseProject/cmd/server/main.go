// Package main is the entry point of the Pulse binary.
//
// cmd/server/main.go is the "wiring" file — it creates all the pieces
// (config, DB, services, handlers) and connects them together.
// It should contain as LITTLE logic as possible; it just assembles and starts.
//
// Think of it like:
//   Python:  if __name__ == "__main__": uvicorn.run(app, host="0.0.0.0", port=8080)
//   Node.js: app.listen(PORT, () => console.log(`Pulse running on ${PORT}`))
package main

import (
	"context"   // context.WithTimeout — gives our shutdown a time limit
	"fmt"       // fmt.Printf / fmt.Fprintf — print to stdout/stderr (replaced by zerolog in Stage 2)
	"net/http"  // http.Server, http.StatusOK, http.ErrServerClosed
	"os"        // os.Signal, os.Stderr, os.Exit
	"os/signal" // signal.Notify — subscribe to OS signals
	"syscall"   // syscall.SIGTERM, syscall.SIGINT — OS signal constants
	"time"      // time.Second — timeout durations

	"github.com/go-chi/chi/v5"            // chi — lightweight HTTP router
	chiMW "github.com/go-chi/chi/v5/middleware" // chi's built-in middleware

	// Our own packages — note they are BELOW cmd/ in the dependency tree.
	"github.com/nishantks908/pulse/config"
)

func main() {
	// ── 1. CONFIG ────────────────────────────────────────────────────────────
	// Read ALL configuration from environment variables at startup.
	// We call Load() ONCE here and pass cfg down to everything that needs it.
	// "One call, many uses" — we never call config.Load() inside a handler.
	//
	// Python: settings = Settings()   # pydantic BaseSettings
	// Node.js: const config = require('./config')
	cfg := config.Load()

	// ── 2. ROUTER ────────────────────────────────────────────────────────────
	// chi.NewRouter() creates a new HTTP mux (multiplexer).
	// It routes incoming HTTP requests to the right handler function.
	//
	// Python/FastAPI:   app = FastAPI()
	// Python/Flask:     app = Flask(__name__)
	// Node.js/Express:  const app = express()
	r := chi.NewRouter()

	// ── 3. GLOBAL MIDDLEWARE ─────────────────────────────────────────────────
	// r.Use() registers middleware that runs for EVERY request.
	// Middleware forms a chain: Request → MW1 → MW2 → Handler → MW2 → MW1 → Response
	// (each middleware can run code both before AND after the handler)
	//
	// Python: app.add_middleware(SomeMiddleware)
	// Node.js: app.use(someMiddleware)

	// Logger: prints one line per request: "GET /health HTTP/1.1 200 1.2ms"
	// Very useful during development; in production we'll replace with zerolog.
	r.Use(chiMW.Logger)

	// Recoverer: if any handler panics (e.g., nil pointer dereference),
	// Recoverer catches the panic, logs the stack trace, and returns HTTP 500.
	// Without Recoverer, a single panic crashes the ENTIRE process.
	//
	// GOTCHA: in Go, a panic in a goroutine kills that goroutine. If the goroutine
	// is your HTTP handler, the whole server process terminates. Always recover!
	//
	// Python: Flask/Django/FastAPI catch panics (exceptions) automatically.
	// Node.js: uncaughtException or express-async-errors handles this.
	r.Use(chiMW.Recoverer)

	// ── 4. ROUTES ────────────────────────────────────────────────────────────
	// r.Get(path, handlerFunc) maps GET requests to a function.
	//
	// Handler signature in Go is ALWAYS:
	//   func(w http.ResponseWriter, r *http.Request)
	//   w = write INTO this (set headers, status code, body)
	//   r = read FROM this (URL params, headers, body)
	//
	// Python/FastAPI: @app.get("/health")
	// Node.js/Express: app.get('/health', (req, res) => { ... })

	// /health is the liveness probe — Kubernetes pings this to check if the
	// pod is alive. Return 200 if the server is running.
	r.Get("/health", healthHandler)

	// More routes will be added in later stages:
	// r.Post("/auth/register", authHandler.Register)
	// r.Post("/auth/login",    authHandler.Login)
	// r.Route("/monitors", func(r chi.Router) { ... })

	// ── 5. HTTP SERVER ───────────────────────────────────────────────────────
	// We create *http.Server explicitly (instead of http.ListenAndServe())
	// because only an explicit *http.Server gives us the Shutdown() method
	// for graceful termination.
	//
	// Python: uvicorn manages the server lifecycle; you rarely do this manually.
	// Node.js: const server = http.createServer(app) then server.close()
	srv := &http.Server{
		Addr:    ":" + cfg.Port, // ":8080" — all network interfaces, port 8080
		Handler: r,              // our chi router handles every request

		// Timeouts defend against slow/malicious clients (e.g., Slowloris attack).
		// Without timeouts, a client that trickles headers one byte per second
		// can hold open a goroutine indefinitely, exhausting your server.
		ReadTimeout:  10 * time.Second, // max time to read headers + body
		WriteTimeout: 10 * time.Second, // max time to send the full response
		IdleTimeout:  60 * time.Second, // max time a keep-alive connection may sit idle
	}

	// ── 6. START SERVER (non-blocking) ───────────────────────────────────────
	// We start the server in a goroutine so main() can continue to the
	// signal-listening code below.
	//
	// If we called srv.ListenAndServe() directly here (without "go"),
	// main() would block at that line and NEVER reach the shutdown logic.
	//
	// "go func() { ... }()" is an immediately-invoked goroutine.
	// goroutine = lightweight cooperative thread managed by the Go runtime.
	//
	// Python/asyncio:   asyncio.create_task(server.serve())
	// Node.js:          app.listen() is non-blocking by default
	//
	// GOTCHA: If the server fails to start (e.g., port already in use),
	// the error happens INSIDE this goroutine, not in main(). We must handle
	// it inside the goroutine — it won't bubble up to the caller automatically.
	go func() {
		fmt.Printf("🚀 Pulse listening on :%s\n", cfg.Port)

		// ListenAndServe blocks until the server is stopped.
		// srv.Shutdown() causes it to return http.ErrServerClosed — that is
		// NOT a real error, it just means "shutdown was requested". We check
		// for it explicitly so we don't log a false alarm.
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Real failure: e.g., "bind: address already in use"
			fmt.Fprintf(os.Stderr, "server failed to start: %v\n", err)
			os.Exit(1) // crash fast on startup failure — retrying makes no sense
		}
	}()

	// ── 7. WAIT FOR SHUTDOWN SIGNAL ──────────────────────────────────────────
	// The server is running in the goroutine above.
	// Now main() waits here until the OS sends SIGTERM or SIGINT.
	//
	// SIGTERM: sent by Kubernetes / systemd when they want to stop the process.
	// SIGINT:  sent when you press Ctrl+C in the terminal.
	//
	// Python: signal.signal(signal.SIGTERM, lambda s, f: shutdown())
	// Node.js: process.on('SIGTERM', () => server.close(() => process.exit(0)))

	// make(chan os.Signal, 1) creates a BUFFERED channel with capacity 1.
	// Why buffered? If the OS delivers the signal before main() reaches <-quit,
	// an unbuffered channel would cause the signal to be dropped (missed!).
	// A buffer of 1 holds the signal safely until we're ready to receive it.
	quit := make(chan os.Signal, 1)

	// Tell the Go runtime: "when you receive SIGTERM or SIGINT, send it to 'quit'."
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	// <-quit BLOCKS main() here — it sleeps until a signal arrives.
	// This is idiomatic Go for "wait forever until told to stop".
	<-quit

	fmt.Println("\n⏳ Shutting down (max 10s)…")

	// ── 8. GRACEFUL SHUTDOWN ─────────────────────────────────────────────────
	// context.WithTimeout creates a Context that auto-cancels after 10 seconds.
	// We pass this to Shutdown(), which means:
	//   - Stop accepting NEW connections immediately.
	//   - Wait up to 10 seconds for IN-FLIGHT requests to finish.
	//   - If 10 seconds pass and some requests are still running, force-close.
	//
	// context.Background() is the root context — no deadline, no cancel, just a root.
	// We derive a child context with a deadline using WithTimeout.
	//
	// defer cancel() ensures context resources are freed when main() returns,
	// even if shutdown finishes in 1 second (defer runs at function exit).
	// This pattern is idiomatic Go — like Python's "with" or Node's "finally".
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// srv.Shutdown(ctx) is the graceful stop:
	//   1. Closes the listener (no new connections).
	//   2. Waits for active connections to become idle (or ctx to expire).
	//   3. Returns nil on clean shutdown, ctx.Err() if timed out.
	if err := srv.Shutdown(ctx); err != nil {
		// Timed out — some connections were force-closed.
		fmt.Fprintf(os.Stderr, "⚠️  shutdown timed out: %v\n", err)
	} else {
		fmt.Println("✅ Pulse stopped cleanly.")
	}
	// main() returns → the process exits with code 0.
}

// ─────────────────────────────────────────────────────────────────────────────
// healthHandler
// ─────────────────────────────────────────────────────────────────────────────

// healthHandler responds to GET /health with a simple JSON payload.
//
// Separating it from main() as a named function makes unit testing easy:
//   w := httptest.NewRecorder()
//   r := httptest.NewRequest(http.MethodGet, "/health", nil)
//   healthHandler(w, r)
//   assert.Equal(t, 200, w.Code)
//
// Python/FastAPI:   async def health() -> dict: return {"status": "ok"}
// Node.js/Express:  app.get('/health', (req, res) => res.json({ status: 'ok' }))
func healthHandler(w http.ResponseWriter, r *http.Request) {
	// Set the Content-Type header BEFORE calling WriteHeader.
	//
	// GOTCHA: Once WriteHeader() is called, headers are flushed to the client
	// and CANNOT be modified. Always set headers first.
	//
	// In Express:  res.json() sets Content-Type automatically.
	// In FastAPI:  JSONResponse sets it automatically.
	// In Go:       you set it manually — more control, more responsibility.
	w.Header().Set("Content-Type", "application/json")

	// Send the HTTP status line: "HTTP/1.1 200 OK"
	// http.StatusOK is the constant 200 — prefer named constants over magic numbers.
	w.WriteHeader(http.StatusOK)

	// Write the response body as raw bytes.
	// []byte("...") converts the string literal to a byte slice —
	// http.ResponseWriter.Write requires []byte, not string.
	//
	// We don't check the error from Write here. If the client closed the
	// connection, there's nothing we can do about it. This is one of the rare
	// "intentionally ignored error" cases in Go. The //nolint comment silences
	// linters that would otherwise flag the ignored return value.
	w.Write([]byte(`{"status":"ok","service":"pulse"}`)) //nolint:errcheck
}
