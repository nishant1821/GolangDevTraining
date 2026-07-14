package cache

// cache.go — Stage 8.
//
// Cache wraps the Redis client with a minimal, purpose-built API.
//
// WHY wrap go-redis instead of using it directly?
//   - ErrCacheMiss: go-redis returns redis.Nil for "key not found".
//     That's a library-specific type; if we ever swap Redis for Memcached
//     or an in-memory map in tests, every caller would need to change.
//     By wrapping, we expose ONE error: ErrCacheMiss — callers need never
//     import go-redis just to check "is this a miss?".
//   - Future-proof: adding instrumentation (metrics, tracing) in one place.
//   - Testability: in tests you can pass a fake Cache with the same interface.
//
// API surface:
//   Get    — fetch a string value; returns ErrCacheMiss on missing key
//   Set    — store any value (JSON-serialisable) with a TTL
//   Del    — remove one or more keys (used on write / invalidation)
//   Incr   — atomic counter increment (used by rate limiter)
//   Expire — set / reset a key's TTL (used by rate limiter on first INCR)
//
// Python analogy: a thin redis.StrictRedis wrapper class
// Node.js analogy: a class wrapping ioredis with translated errors
//
// Cache-aside pattern recap (the only pattern this package enables):
//
//   Read path:
//     cache.Get(key) → hit → return cached value          (DB never touched)
//                   → miss → fetch DB → cache.Set(key, v, TTL) → return v
//
//   Write path:
//     update DB → cache.Del(key)   ← key removed; next read repopulates it
//
// Why Del (evict) instead of Set (update) on writes?
//   If we Set the new value but the DB write fails, cache has stale data.
//   Del is the safe default: at worst, the next read does one extra DB query.

import (
	"context"  // context.Context — every Redis call is cancellable
	"errors"   // errors.Is — check for redis.Nil
	"fmt"      // fmt.Errorf — wrap errors with context
	"time"     // time.Duration — TTL values

	"github.com/redis/go-redis/v9" // go-redis v9 — Redis client for Go
)

// ─────────────────────────────────────────────────────────────────────────────
// Sentinel error
// ─────────────────────────────────────────────────────────────────────────────

// ErrCacheMiss is returned by Get when the key does not exist in Redis
// (or has expired). Callers use errors.Is(err, cache.ErrCacheMiss) to
// distinguish "key not in cache" from a real Redis network error.
//
// Python: raise CacheMiss(key) — custom exception
// Node.js: throw new CacheMissError(key)
// Go:     return "", cache.ErrCacheMiss
var ErrCacheMiss = errors.New("cache miss")

// ─────────────────────────────────────────────────────────────────────────────
// Cache — the wrapper struct
// ─────────────────────────────────────────────────────────────────────────────

// Cache holds the go-redis client and exposes the operations Pulse needs.
// Unexported client — callers interact through the methods, not raw Redis.
type Cache struct {
	client *redis.Client
}

// New connects to Redis using the given URL and returns a ready-to-use Cache.
// It sends a PING to verify the connection; returns an error if Redis is down.
//
// URL format: redis://[:password@]host[:port][/db]
//   e.g., "redis://localhost:6379"
//        "redis://:secret@redis:6379/0"
//
// We return an error (not panic) so main.go can decide to degrade gracefully
// (run without cache) rather than crashing the whole service.
//
// Python: redis.from_url(url)  — StrictRedis.from_url
// Node.js: new Redis(url)      — ioredis constructor
func New(redisURL string) (*Cache, error) {
	// redis.ParseURL turns the URL string into an *redis.Options struct.
	// It handles auth, DB index, TLS flags embedded in the URL.
	//
	// Python: redis.ConnectionPool.from_url(url)
	// Node.js: ioredis parses the URL automatically in the constructor
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("cache: parse url: %w", err)
	}

	// redis.NewClient creates the client — it does NOT connect yet.
	// The first command triggers the actual TCP connection (lazy connect).
	client := redis.NewClient(opts)

	// PING verifies the connection is alive at startup.
	// This is the "fail fast" principle: if Redis is unreachable, we know
	// immediately (before the first request) rather than at request time.
	//
	// context.Background() — no deadline for the startup ping.
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("cache: ping failed: %w", err)
	}

	return &Cache{client: client}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Get — fetch a string value
// ─────────────────────────────────────────────────────────────────────────────

// Get retrieves the value for key as a string.
// Returns ErrCacheMiss if the key doesn't exist or has expired.
// Returns a non-miss error only on network/Redis failures.
//
// CALLER PATTERN:
//   val, err := c.Get(ctx, "pulse:monitor:42")
//   if errors.Is(err, cache.ErrCacheMiss) {
//       // cache miss — go to DB
//   } else if err != nil {
//       // real Redis error — log and go to DB as fallback
//   }
//   // use val
//
// Python: cache.get(key)  — returns None on miss
// Node.js: await client.get(key)  — returns null on miss
// Go:     returns ("", ErrCacheMiss) — explicit miss signal
func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		// redis.Nil means "key not found" — translate to our domain error.
		// We NEVER expose redis.Nil outside this package.
		return "", ErrCacheMiss
	}
	if err != nil {
		return "", fmt.Errorf("cache get %q: %w", key, err)
	}
	return val, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Set — store a value with a TTL
// ─────────────────────────────────────────────────────────────────────────────

// Set stores value under key with the given TTL.
// When TTL expires, Redis automatically removes the key — no cron needed.
//
// value can be a string, []byte, or any type that Redis knows how to encode.
// For structs, pass the JSON-marshalled string (caller's responsibility).
//
// TTL=0 means "no expiry" (use with caution — cache can grow unbounded).
//
// Python: cache.setex(key, ttl_seconds, value)
// Node.js: await client.set(key, value, 'EX', ttl_seconds)
func (c *Cache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if err := c.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("cache set %q: %w", key, err)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Del — delete one or more keys
// ─────────────────────────────────────────────────────────────────────────────

// Del removes the given keys from Redis. Called on writes (cache invalidation).
// Deleting a non-existent key is NOT an error — it's idempotent.
//
// Python: cache.delete(*keys)
// Node.js: await client.del(...keys)
func (c *Cache) Del(ctx context.Context, keys ...string) error {
	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache del: %w", err)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Incr — atomic counter increment
// ─────────────────────────────────────────────────────────────────────────────

// Incr atomically increments the integer at key by 1 and returns the new value.
// If the key doesn't exist, Redis creates it with value 0 then increments to 1.
//
// WHY INCR is safe for rate limiting (no mutex needed):
//   Redis is single-threaded for command execution.
//   INCR is a single atomic operation — read-modify-write happens in Redis itself.
//
//   Problematic GET + SET race:
//     goroutine A: GET key → 0
//     goroutine B: GET key → 0     ← both see 0 because A hasn't written yet
//     goroutine A: SET key 1
//     goroutine B: SET key 1       ← overwrites A — only 1 counted instead of 2
//
//   With INCR:
//     goroutine A: INCR key → 1   ← atomic: read+write together in Redis
//     goroutine B: INCR key → 2   ← sees A's write; correctly counts 2
//
// Python: pipe.incr(key)  — or redis.incr(key)
// Node.js: await client.incr(key)
func (c *Cache) Incr(ctx context.Context, key string) (int64, error) {
	n, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("cache incr %q: %w", key, err)
	}
	return n, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Expire — set / reset a key's TTL
// ─────────────────────────────────────────────────────────────────────────────

// Expire sets the TTL on an existing key.
// In the rate limiter this is called after the FIRST Incr (count==1) to start
// the expiry clock. We call it conditionally instead of always so we don't
// reset the window on every request.
//
// WHY only on count==1?
//   If we called Expire on every request, the window would never expire while
//   traffic is flowing. count==1 means "first hit in this window — start the clock".
//   The key expires `window` seconds later, opening a new window.
//
//   Window bucket approach (alternative): use time.Now()/window as key suffix.
//   Both work; "EXPIRE on first INCR" is simpler to understand.
//
// Python: cache.expire(key, ttl_seconds)
// Node.js: await client.expire(key, ttl_seconds)
func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if err := c.client.Expire(ctx, key, ttl).Err(); err != nil {
		return fmt.Errorf("cache expire %q: %w", key, err)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Ping — liveness check
// ─────────────────────────────────────────────────────────────────────────────

// Ping sends a PING command and returns an error if Redis is unreachable.
// Used by the /health endpoint to report Redis liveness.
//
// The context allows a deadline:
//   ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
//   defer cancel()
//   err := cache.Ping(ctx)   // returns within 2s or ctx.Err()
//
// Python: client.ping()   — raises ConnectionError if Redis is down
// Node.js: await client.ping()   — rejects if down
func (c *Cache) Ping(ctx context.Context) error {
	if err := c.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cache ping: %w", err)
	}
	return nil
}
