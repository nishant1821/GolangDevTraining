package middleware

// ratelimit.go — Stage 8.
//
// RateLimit middleware uses Redis INCR + EXPIRE to count requests per client
// and return 429 Too Many Requests once the limit is exceeded.
//
// ALGORITHM — fixed window counter:
//
//   Key: "pulse:rl:{ip}:{window_bucket}"
//   where window_bucket = floor(Unix() / window_seconds)
//
//   On every request:
//     1. INCR the key                     → count (atomic)
//     2. If count == 1: EXPIRE key window → start the expiry clock
//     3. If count > limit: return 429
//     4. Else: set rate-limit headers, call next handler
//
// WHY Redis INCR is correct (not GET + conditional SET):
//
//   GET + SET race condition:
//     goroutine A: GET "pulse:rl:1.2.3.4:123" → 5
//     goroutine B: GET "pulse:rl:1.2.3.4:123" → 5   ← sees same value!
//     goroutine A: SET key 6
//     goroutine B: SET key 6                          ← overwrites A; only 1 counted
//
//   INCR is one atomic operation inside Redis (single-threaded execution):
//     goroutine A: INCR → 6  ← Redis reads 5, writes 6, returns 6
//     goroutine B: INCR → 7  ← Redis reads 6, writes 7, returns 7  ✓ both counted
//
//   No mutex needed in Go — the atomicity lives in Redis.
//
// WHY EXPIRE only on count==1 (not every request)?
//   If we called EXPIRE on every request while traffic is flowing,
//   the key would never expire (window keeps getting pushed forward).
//   count==1 = "first hit in a new window" → start the expiry clock once.
//   Window ends naturally when the key TTL fires.
//
// Fail-open policy:
//   If Redis is unavailable, INCR errors → we allow the request through.
//   Better to serve too many requests than to block legitimate traffic
//   because the rate-limiter's backing store is down.
//
// Python/Flask-Limiter:
//   @limiter.limit("100/minute")
//   def my_view(): ...
//
// Node.js/express-rate-limit:
//   app.use(rateLimit({ windowMs: 60_000, max: 100 }))
//
// Go/chi (this file):
//   r.Use(middleware.RateLimit(cacheClient, 100, time.Minute))

import (
	"fmt"      // fmt.Sprintf — build the rate-limit key
	"net"      // net.SplitHostPort — extract IP from RemoteAddr
	"net/http" // http.Handler, http.ResponseWriter, http.StatusTooManyRequests
	"strconv"  // strconv.FormatInt — convert window bucket to string
	"time"     // time.Duration, time.Now, time.Unix

	"github.com/nishantks908/pulse/internal/platform/cache" // cache.Cache
)

// RateLimit returns a chi-compatible middleware.
//
// Parameters:
//   c      — Redis cache client (can be nil; middleware fails-open when nil)
//   limit  — max requests per window per client IP
//   window — the rolling window duration (e.g., time.Minute)
//
// Usage:
//   r.Use(middleware.RateLimit(cacheClient, 100, time.Minute))
func RateLimit(c *cache.Cache, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			// ── Fail-open if cache unavailable ────────────────────────────────
			// If Redis is nil (not configured or startup failed), skip rate
			// limiting entirely. Never block traffic because of a missing cache.
			if c == nil {
				next.ServeHTTP(w, r)
				return
			}

			// ── Build the rate-limit key ──────────────────────────────────────
			// Key = "pulse:rl:{client-ip}:{window-bucket}"
			//
			// window-bucket: divides Unix time into fixed-size buckets.
			//   window=60s, now=1705234567 → bucket = 1705234567/60 = 28420576
			//   All requests in the same 60s bucket share the same key.
			//   Next 60s bucket: 28420577 → new key → fresh counter.
			//
			// clientIP: we parse r.RemoteAddr which is "ip:port".
			// In production behind a proxy, use X-Forwarded-For or X-Real-IP.
			// For Stage 8, RemoteAddr is sufficient.
			//
			// Python: key = f"pulse:rl:{request.remote_addr}:{int(time.time() // 60)}"
			// Node.js: const key = `pulse:rl:${req.ip}:${Math.floor(Date.now()/60000)}`
			ip := clientIP(r)
			bucketSize := int64(window.Seconds())
			bucket := time.Now().Unix() / bucketSize
			key := fmt.Sprintf("pulse:rl:%s:%s", ip, strconv.FormatInt(bucket, 10))

			// ── INCR — atomic counter increment ──────────────────────────────
			// If key doesn't exist, Redis creates it at 0 then returns 1.
			// Multiple concurrent requests all INCR safely — no race condition.
			count, err := c.Incr(r.Context(), key)
			if err != nil {
				// Redis error → fail-open: allow the request.
				next.ServeHTTP(w, r)
				return
			}

			// ── EXPIRE — start the expiry clock on the first hit ──────────────
			// count==1 means "first request in this window bucket".
			// We set the TTL exactly once — it expires after `window` seconds.
			// Subsequent requests in the same bucket increment the counter
			// but do NOT reset the expiry.
			//
			// If we called EXPIRE every time, a sustained burst would keep
			// pushing the expiry forward and the window would never close.
			if count == 1 {
				// Slightly longer TTL (window + 5s) to handle clock skew between
				// app servers. Without the buffer, a request at the very end of a
				// bucket might arrive at Redis slightly after the key expires,
				// creating a phantom fresh window.
				if err := c.Expire(r.Context(), key, window+5*time.Second); err != nil {
					// Non-fatal: key will be cleaned up by Redis LRU eviction.
				}
			}

			// ── Rate-limit headers ────────────────────────────────────────────
			// Standard headers (de-facto convention, from IETF draft):
			//   X-RateLimit-Limit:     max requests allowed per window
			//   X-RateLimit-Remaining: requests left in current window
			//   X-RateLimit-Reset:     Unix timestamp when the window resets
			//
			// These let API clients back off before hitting 429.
			remaining := int64(limit) - count
			if remaining < 0 {
				remaining = 0
			}
			// Window resets at the start of the NEXT bucket.
			resetAt := (bucket + 1) * bucketSize

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt, 10))

			// ── Enforce the limit ─────────────────────────────────────────────
			if count > int64(limit) {
				// Retry-After: how many seconds until the current window expires.
				retryAfter := resetAt - time.Now().Unix()
				w.Header().Set("Retry-After", strconv.FormatInt(retryAfter, 10))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests) // 429
				// Include request_id in the error response.
				reqID := RequestIDFromCtx(r.Context())
				fmt.Fprintf(w, //nolint:errcheck
					`{"success":false,"error":"rate_limit_exceeded","message":"too many requests — retry after %ds","request_id":%q}`,
					retryAfter, reqID,
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// clientIP extracts the client IP address from r.RemoteAddr.
// r.RemoteAddr is always "ip:port" — we strip the port.
// Returns the raw string if parsing fails (defensive).
func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr // fallback: return as-is
	}
	return ip
}
