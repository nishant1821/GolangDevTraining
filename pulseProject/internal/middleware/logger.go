package middleware

// logger.go — Stage 7.
//
// NewLogger returns a zerolog request-logging middleware.
//
// zerolog is a structured, zero-allocation JSON logger. Every log line is a
// JSON object — machine-readable, index-friendly (Datadog, Loki, CloudWatch).
//
//   {"level":"info","request_id":"abc","method":"POST","path":"/api/monitors",
//    "status":201,"bytes":312,"latency_ms":4,"time":"2025-07-14T12:00:00Z"}
//
// WHY zerolog over fmt.Println / log.Printf?
//   - Structured → grep/filter by field (e.g., all 5xx responses)
//   - Zero-allocation hot path → no GC pressure in high-throughput services
//   - Level filtering → disable DEBUG in prod without code changes
//
// Python analogy: structlog or python-json-logger
//   import structlog
//   log = structlog.get_logger()
//   log.info("request", method="POST", path="/monitors", status=201)
//
// Node.js analogy: pino
//   const log = pino()
//   log.info({ method: 'POST', path: '/monitors', status: 201 }, 'request')
//
// Middleware ordering rule:
//
//   r.Use(middleware.RequestID)   ← must be FIRST
//   r.Use(middleware.NewLogger(log))  ← second: reads request ID set by #1
//   r.Use(chiMW.Recoverer)        ← third: catches panics from handlers
//
// On the way IN (outermost to innermost):
//   RequestID → Logger → Recoverer → Handler
//
// On the way OUT (innermost to outermost, LIFO stack):
//   Handler → Recoverer → Logger (captures status + duration) → RequestID
//
// The Logger wraps the ResponseWriter so it can intercept WriteHeader() and
// capture the HTTP status code AFTER the handler writes it.

import (
	"net/http" // http.Handler, http.ResponseWriter
	"time"     // time.Since — measure request duration

	"github.com/rs/zerolog" // zerolog.Logger

	// No import of handler package — middleware must never import handler
	// (that would be a circular import: handler → middleware → handler).
)

// ─────────────────────────────────────────────────────────────────────────────
// responseWriter — captures status code and bytes written
// ─────────────────────────────────────────────────────────────────────────────

// responseWriter wraps http.ResponseWriter to intercept WriteHeader and Write.
// The standard http.ResponseWriter doesn't expose the status code after the
// fact — we capture it here so the logger can record it AFTER the handler runs.
//
// Python/WSGI: similar to a middleware that wraps start_response to capture status.
// Node.js/Express: monkey-patching res.json / res.send — same concept.
// Go:             embed the interface and override the two write methods.
type responseWriter struct {
	http.ResponseWriter        // embed — inherits all other methods (Header, Flush, etc.)
	status             int     // captured HTTP status code (default 200)
	written            int     // total bytes written to body
	wroteHeader        bool    // guard: WriteHeader must only fire once
}

// WriteHeader intercepts the status code before forwarding to the real writer.
func (rw *responseWriter) WriteHeader(status int) {
	if rw.wroteHeader {
		return // guard: chi Recoverer may call this twice on panics
	}
	rw.status = status
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(status)
}

// Write intercepts the body bytes to count them, then writes to the real writer.
// If WriteHeader was never called explicitly, http.ResponseWriter defaults to 200.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		// First Write implicitly sends a 200 — mirror that behaviour.
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.written += n
	return n, err
}

// ─────────────────────────────────────────────────────────────────────────────
// NewLogger — middleware factory
// ─────────────────────────────────────────────────────────────────────────────

// NewLogger returns a chi-compatible middleware that logs every request.
// Takes a zerolog.Logger so you can configure it (pretty vs JSON, level, etc.)
// without touching this file.
//
// Usage:
//   log := zerolog.New(os.Stdout).With().Timestamp().Logger()
//   r.Use(middleware.NewLogger(log))
//
// Python: functools.partial(logging_middleware, logger=logger)
// Node.js: app.use(pinoHttp({ logger: log }))
func NewLogger(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap the ResponseWriter so we can capture the status code.
			// Default status is 200 — matches http.ResponseWriter's implicit default.
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			// Record start time BEFORE calling next — gives accurate latency.
			start := time.Now()

			// ── Call the rest of the middleware chain + handler ───────────────
			// Everything downstream runs HERE. When next.ServeHTTP returns,
			// the handler has already written its response — rw.status is set.
			next.ServeHTTP(rw, r)

			// ── After handler returns: log the completed request ──────────────
			// RequestIDFromCtx reads the UUID that RequestID middleware stored.
			// This works because RequestID runs BEFORE Logger in the chain —
			// so by the time we get here, the context already has the UUID.
			//
			// Python: request.state.request_id  (set by RequestID middleware above)
			// Node.js: req.requestId             (same concept)
			reqID := RequestIDFromCtx(r.Context())

			// Choose log level based on status code:
			//   5xx → Error (actionable, page someone)
			//   4xx → Warn  (client error, informational)
			//   2xx/3xx → Info (normal operation)
			//
			// zerolog uses a builder pattern:
			//   log.Info()  → *zerolog.Event
			//   .Str("k","v")  → add string field
			//   .Int("k", n)   → add int field
			//   .Msg("text")   → write the event (MUST call this to flush)
			event := logEvent(log, rw.status)
			event.
				Str("request_id", reqID).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", rw.status).
				Int("bytes", rw.written).
				Int64("latency_ms", time.Since(start).Milliseconds()).
				Msg("request")
		})
	}
}

// logEvent picks the zerolog level based on HTTP status code.
// Returns a *zerolog.Event ready to chain field setters onto.
func logEvent(log zerolog.Logger, status int) *zerolog.Event {
	switch {
	case status >= 500:
		return log.Error()
	case status >= 400:
		return log.Warn()
	default:
		return log.Info()
	}
}
