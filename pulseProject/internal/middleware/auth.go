package middleware

// auth.go — Stage 7.
//
// Auth middleware extracts the Bearer token from the Authorization header,
// validates it (signature + expiry), and stores the UserID in the request context.
//
// Every protected route goes through this middleware BEFORE the handler runs.
// If the token is missing, expired, or tampered with → 401 Unauthorized.
// If valid → the handler can call UserIDFromCtx(r.Context()) to get the UserID.
//
// Middleware chain:
//
//   Request → Auth middleware → handler
//              (validates JWT,
//               injects UserID)
//
// Python/FastAPI:
//   async def get_current_user(token = Depends(oauth2_scheme)): ...
//   @router.get("/monitors", dependencies=[Depends(get_current_user)])
//
// Node.js/Express:
//   function authMiddleware(req, res, next) {
//     const token = req.headers.authorization?.split(" ")[1]
//     const payload = jwt.verify(token, secret)
//     req.userId = payload.userId
//     next()
//   }
//   router.use("/monitors", authMiddleware)
//
// Go/chi:
//   r.Route("/monitors", func(r chi.Router) {
//     r.Use(middleware.Auth(jwtSecret))   ← this file
//     r.Get("/", handler.List)
//   })

import (
	"context"      // context.WithValue, context.Context
	"errors"       // errors.Is — check specific JWT errors
	"net/http"     // http.Handler, http.Request, http.ResponseWriter
	"strings"      // strings.HasPrefix, strings.TrimPrefix

	"github.com/golang-jwt/jwt/v5" // jwt.ParseWithClaims

	"github.com/nishantks908/pulse/internal/service" // service.PulseClaims
)

// ─────────────────────────────────────────────────────────────────────────────
// Context key type — avoids collisions with other packages
// ─────────────────────────────────────────────────────────────────────────────

// ctxKey is a private type used as the key when storing values in context.
//
// WHY a custom type and not just a string?
//   context.WithValue(ctx, "userID", 5) — BAD.
//   Two packages could both use the string key "userID" and overwrite each other.
//   A package-private type is unique by definition — no collisions possible.
//
// Python: no direct equivalent; usually done with Request.state or g in Flask.
// Node.js: res.locals.userId or req.userId (no collision protection built in).
// Go:     custom unexported type → compile-time uniqueness guaranteed.
type ctxKey string

const userIDKey ctxKey = "userID" // the key used to store/retrieve UserID in context

// ─────────────────────────────────────────────────────────────────────────────
// Auth — the middleware factory
// ─────────────────────────────────────────────────────────────────────────────

// Auth returns an http.Handler middleware that:
//  1. Reads the "Authorization: Bearer <token>" header.
//  2. Parses and validates the JWT (signature + expiry).
//  3. Injects the UserID into the request context.
//  4. Calls next.ServeHTTP if valid; writes 401 if not.
//
// It's a "factory" (a function that returns a function) so we can inject
// jwtSecret at startup time (from config) without a global variable.
//
// Python: functools.partial(auth_middleware, secret=secret)
// Node.js: const auth = (secret) => (req, res, next) => { … }   (closure)
// Go:     func Auth(secret) func(http.Handler) http.Handler      (same concept)
func Auth(jwtSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			// ── Step 1: Extract the token from the header ─────────────────────
			// HTTP convention for Bearer tokens:
			//   Authorization: Bearer eyJhbGci...
			//
			// If the header is absent or malformed → 401.
			//
			// Python: token = request.headers.get("Authorization", "").removeprefix("Bearer ")
			// Node.js: const [, token] = (req.headers.authorization || "").split(" ")
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, `{"error":"missing or invalid authorization header"}`, http.StatusUnauthorized)
				return
			}
			// TrimPrefix strips exactly "Bearer " — leaves the raw token string.
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			// ── Step 2: Parse and validate the JWT ────────────────────────────
			// jwt.ParseWithClaims does three things automatically:
			//   a. Decodes the base64url-encoded header + payload.
			//   b. Verifies the signature using the provided key.
			//   c. Checks expiry (exp claim) and other standard claims.
			//
			// The key function receives the unverified token and must return the
			// secret used to verify it. This indirection allows multi-key setups
			// (e.g., key rotation); here we always return jwtSecret.
			//
			// Python: jwt.decode(token, secret, algorithms=["HS256"])
			// Node.js: jwt.verify(token, secret)
			claims := &service.PulseClaims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
				// Guard: ensure the signing method is HMAC, not "none" or RSA.
				// An attacker could forge a token with algorithm="none" and no signature
				// if this check is missing — the "none algorithm" attack.
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, errors.New("unexpected signing method")
				}
				return jwtSecret, nil
			})

			if err != nil || !token.Valid {
				// Distinguish expiry from other errors for a clearer message.
				// errors.Is traverses the error chain — works even through fmt.Errorf wrapping.
				msg := `{"error":"invalid token"}`
				if errors.Is(err, jwt.ErrTokenExpired) {
					msg = `{"error":"token expired"}`
				}
				http.Error(w, msg, http.StatusUnauthorized)
				return
			}

			// ── Step 3: Inject UserID into the context ────────────────────────
			// context.WithValue creates a NEW context with the key-value pair.
			// The original context r.Context() is NOT mutated — contexts are immutable.
			//
			// Handlers downstream call UserIDFromCtx(r.Context()) to retrieve this.
			//
			// Python/FastAPI: request.state.user_id = claims.user_id; await call_next(request)
			// Node.js/Express: req.userId = payload.userId; next()
			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// UserIDFromCtx — helper for handlers
// ─────────────────────────────────────────────────────────────────────────────

// UserIDFromCtx retrieves the UserID stored by the Auth middleware.
// Returns (0, false) if no UserID was found — meaning Auth middleware
// was not in the chain (developer error) or the context was wrong.
//
// Usage in a handler:
//
//	userID, ok := middleware.UserIDFromCtx(r.Context())
//	if !ok { http.Error(w, "unauthorized", 401); return }
//
// Python: user_id = request.state.user_id
// Node.js: const userId = req.userId
func UserIDFromCtx(ctx context.Context) (uint, bool) {
	// ctx.Value returns any (interface{}); we type-assert to uint.
	// The comma-ok pattern: if the key is absent or wrong type, ok=false.
	id, ok := ctx.Value(userIDKey).(uint)
	return id, ok
}

// WithUserID returns a context with userID stored under the same key that
// Auth middleware uses. Intended for handler tests that bypass real JWT auth.
//
// WHY exported for tests?
//   The ctxKey type is unexported — tests in other packages can't create a
//   value of that type to store directly. This helper is the safe crossing
//   point: tests call WithUserID; only this package knows the key type.
//
// Usage in a handler test:
//   r = r.WithContext(middleware.WithUserID(r.Context(), 42))
//   handlerFn(w, r)  // handler sees userID=42 in ctx, no real JWT needed
//
// Python/FastAPI: app.dependency_overrides[current_user] = lambda: FakeUser(id=42)
// Node.js/Express: req.userId = 42  (set manually in test before calling handler)
func WithUserID(ctx context.Context, userID uint) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}
