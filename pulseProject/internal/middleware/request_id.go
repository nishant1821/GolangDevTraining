package middleware

// request_id.go — Stage 7.
//
// RequestID middleware attaches a unique UUID to every incoming request.
//
// WHY a request ID?
//   In production, many requests arrive in parallel. Logs interleave.
//   Without a request ID it is impossible to group all log lines for
//   one request together. With it:
//
//     {"request_id":"abc","method":"POST","path":"/api/monitors","status":422}
//     {"request_id":"abc","message":"url is required"}   ← same request!
//
//   Every log line + every error response carries the same ID — support
//   engineers can grep for it and see the full picture instantly.
//
// Flow:
//   1. Client sends request (no X-Request-ID header usually).
//   2. Middleware generates UUID v4, stores it in context.
//   3. Middleware writes X-Request-ID response header so the client can
//      include it in bug reports.
//   4. Logger middleware reads it from context (see logger.go).
//   5. respond.go reads it from context and includes it in JSON responses.
//
// Python/FastAPI:
//   @app.middleware("http")
//   async def add_request_id(request: Request, call_next):
//       request_id = str(uuid.uuid4())
//       request.state.request_id = request_id
//       response = await call_next(request)
//       response.headers["X-Request-ID"] = request_id
//       return response
//
// Node.js/Express:
//   app.use((req, res, next) => {
//     const id = uuid()
//     req.requestId = id
//     res.setHeader("X-Request-ID", id)
//     next()
//   })

import (
	"context"  // context.WithValue — stores the ID in the request context
	"net/http" // http.Handler — middleware signature

	"github.com/google/uuid" // uuid.NewString() — generates a v4 UUID
)

// ─────────────────────────────────────────────────────────────────────────────
// Context key — package-private to avoid key collisions
// ─────────────────────────────────────────────────────────────────────────────

// reqIDKey is a private type used as context key for the request ID.
// Using a custom unexported type (not a plain string) prevents any other
// package from accidentally overwriting it with the same key.
//
// Python analogy: no equivalent — Python uses request.state (no collision risk).
// Node.js analogy: Symbol("requestId") — symbol uniqueness protects against key collisions.
// Go:             unexported named type — compile-time uniqueness guaranteed.
type reqIDKey struct{}

// Header is the HTTP header name clients receive and can echo back in bug reports.
const Header = "X-Request-ID"

// ─────────────────────────────────────────────────────────────────────────────
// RequestID — the middleware
// ─────────────────────────────────────────────────────────────────────────────

// RequestID is a chi-compatible middleware (func(http.Handler) http.Handler).
// It generates a UUID v4, stores it in context, and echoes it as a response header.
//
// Usage in main.go:
//   r.Use(middleware.RequestID)
//
// Note: chi also ships its own middleware.RequestID — we build our own so we
// control the context key and can expose RequestIDFromCtx as a clean API.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// uuid.NewString() generates a random UUID v4 string:
		// "550e8400-e29b-41d4-a716-446655440000"
		//
		// Python: str(uuid.uuid4())
		// Node.js: require('crypto').randomUUID()  or  uuid()
		id := uuid.NewString()

		// Store in context — downstream middleware + handlers can read it.
		// context.WithValue returns a NEW context; it does NOT modify r.Context().
		// Contexts in Go are immutable — WithValue is a "copy-on-write" operation.
		ctx := context.WithValue(r.Context(), reqIDKey{}, id)

		// Set response header BEFORE calling next — headers must be set before
		// WriteHeader() or the first Write(), whichever comes first.
		w.Header().Set(Header, id)

		// Call the next handler/middleware with the enriched context.
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromCtx retrieves the request ID from the context.
// Returns "" if RequestID middleware was not in the chain (developer error).
//
// Usage:
//   id := middleware.RequestIDFromCtx(r.Context())
//
// Python: request.state.request_id
// Node.js: req.requestId
func RequestIDFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(reqIDKey{}).(string)
	return id
}
