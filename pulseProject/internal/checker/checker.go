package checker

// checker.go defines the PUBLIC contract of this package:
//   - Result  → what a check produces
//   - Checker → the interface every implementation must satisfy
//
// We separate the interface (this file) from the implementation (http_checker.go)
// so that tests can import only this file and provide a fake implementation
// without pulling in net/http, DNS resolution, or any network I/O.

import (
	"context" // context.Context — carries deadlines and cancellation signals
	"time"    // time.Duration, time.Milliseconds — for latency
)

// ─────────────────────────────────────────────────────────────────────────────
// Result — what one HTTP probe tells us
// ─────────────────────────────────────────────────────────────────────────────

// Result holds every piece of information produced by a single HTTP check.

// Design note: Result is returned BY VALUE (not *Result pointer).
// This is intentional:
//   - It's a small struct (~40 bytes). Copying is cheaper than heap-allocating.
//   - A nil pointer is never returned, so callers never need nil-checks.
//   - Value types are safer in concurrent code (no shared-pointer races).

// Python analogy:  @dataclass  or  namedtuple("Result", ["status_code", "latency_ms", ...])
// Node.js analogy: a plain object  { statusCode: 200, latencyMs: 45, up: true, err: null }
// Go:              a struct returned by value — the compiler copies it on the stack
type Result struct {
	// StatusCode is the HTTP response status.
	// 200 = OK, 503 = Service Unavailable, 0 = no response received (timeout/network error).
	//
	// We use 0 (not -1 or some sentinel) because 0 is the zero-value for int in Go,
	// and 0 is not a valid HTTP status code — so it unambiguously means "no response".
	StatusCode int

	// LatencyMs is the round-trip time in milliseconds from "request sent" to
	// "first byte of response received" (or "error returned").
	// Stored as int64 because time.Duration is int64 nanoseconds internally.
	LatencyMs int64

	// Up is the human-readable summary:
	//   true  → 2xx response within the deadline
	//   false → non-2xx, timeout, DNS failure, or any other error
	//
	// Storing this as a boolean (even though it's derivable from StatusCode)
	// lets consumers do a quick if result.Up {} without knowing HTTP semantics.
	Up bool

	// Err holds the transport-level error when no HTTP response was received.
	// nil means: "we got an HTTP response" (even if StatusCode is 503).
	// non-nil means: "we never got a response" (timeout, DNS failure, etc.)
	//
	// This distinction matters:
	//   Err == nil, Up == false → server is reachable but returning errors (5xx)
	//   Err != nil              → server is completely unreachable
	//
	// Python: requests raises an exception for network errors, returns a Response for HTTP errors.
	//   try: r = requests.get(url) → network error → except requests.ConnectionError
	//   r.status_code == 503       → HTTP error
	// Node.js: fetch rejects for network errors, resolves for HTTP errors (same pattern).
	// Go:      we unify both cases in one Result struct — no exception, no rejected promise.
	Err error
}

// ─────────────────────────────────────────────────────────────────────────────
// Checker — the interface
// ─────────────────────────────────────────────────────────────────────────────

// Checker is the contract that every HTTP-checking implementation must satisfy.
//
// WHY AN INTERFACE AND NOT JUST A STRUCT?
//
// Analogy: a power socket. The wall doesn't care if you plug in a phone charger,
// a laptop charger, or a lamp — as long as the plug fits the socket shape.
// The socket is the interface. Each device is an implementation.
//
// In our case:
//   - The worker pool (Stage 6) calls checker.Check(ctx, url).
//   - It holds a Checker interface, NOT a concrete *HTTPChecker.
//   - In production: the real HTTPChecker makes network calls.
//   - In tests:      a fakeChecker returns a hardcoded Result instantly.
//
// Without an interface, tests would make real HTTP calls → flaky, slow, brittle.
// With an interface, you inject whichever implementation fits the context.
//
// Python: typing.Protocol or ABC (Abstract Base Class)
//
//	class Checker(Protocol):
//	    def check(self, ctx, url: str) -> Result: ...
//
// Node.js/TypeScript:
//
//	interface Checker { check(ctx: AbortSignal, url: string): Promise<Result> }
//
// Go: the interface is defined here; any type with a Check method satisfies it
// automatically — no "implements Checker" declaration needed (duck typing).
//
// CONTEXT AS FIRST PARAMETER:
// This is idiomatic Go. context.Context carries:
//  1. A deadline (how long to wait before giving up)
//  2. A cancellation signal (the caller decided to stop early — e.g., SIGTERM)
//  3. Request-scoped values (e.g., request ID for logging)
//
// You always pass ctx as the FIRST parameter, before anything else.
//
// Python: no equivalent in stdlib; asyncio uses Task.cancel() — separate mechanism.
// Node.js: AbortController / AbortSignal — same concept, different API.
type Checker interface {
	// Check probes url and returns a Result.
	// It NEVER returns an error directly — errors are inside Result.Err.
	// This makes call sites cleaner: you always get a Result, never a panic or nil.
	Check(ctx context.Context, url string) Result
}

// ─────────────────────────────────────────────────────────────────────────────
// Package-level helpers (used by both HTTPChecker and tests)
// ─────────────────────────────────────────────────────────────────────────────

// IsUp returns true if statusCode is in the 2xx range (success).
// Extracted as a function so both HTTPChecker and fake checkers use
// the SAME definition of "up" — no divergence between prod and test logic.
func IsUp(statusCode int) bool {
	// 200–299 is the HTTP "success" range (RFC 9110).
	// We intentionally exclude 3xx redirects — if a URL redirects, the
	// original URL is "technically down" from a pure availability standpoint.
	// (CheckRedirect in HTTPChecker stops following redirects anyway.)
	return statusCode >= 200 && statusCode < 300
}

// newErrorResult builds a failed Result from a pure transport error.
// Used internally when no HTTP response was received at all.
//
// Unexported (lowercase n) — this is an internal helper, not part of the
// public API. Callers of this package use the Checker interface, not this function.
func newErrorResult(latency time.Duration, err error) Result {
	return Result{
		StatusCode: 0, // 0 = no HTTP response received
		LatencyMs:  latency.Milliseconds(),
		Up:         false,
		Err:        err,
	}
}
