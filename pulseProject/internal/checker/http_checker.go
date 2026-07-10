package checker

// http_checker.go is the CONCRETE implementation of the Checker interface.
// It uses Go's standard net/http library to make real HTTP GET requests.
//
// Other packages never import this file directly — they hold a Checker interface.
// Only main.go (the wiring layer) creates an *HTTPChecker and assigns it to a Checker.
// This is called "Dependency Injection" — the consumer declares what it needs
// (Checker interface), the wirer provides the concrete thing (*HTTPChecker).
//
// Python: dependency_injector library, or just passing objects in __init__
// Node.js: constructor injection — new WorkerPool(new HttpChecker())

import (
	"context"  // context.Context — for request cancellation and timeouts
	"fmt"      // fmt.Errorf — to wrap errors with %w for errors.Is() chain
	"io"       // io.Discard — to drain the response body efficiently
	"net/http" // http.Client, http.NewRequestWithContext, http.ErrUseLastResponse
	"time"     // time.Now, time.Since, time.Duration
)

// HTTPChecker satisfies the Checker interface using a real HTTP client.
//
// It is a struct, not an interface — it is the concrete "thing" that does
// actual network I/O. Consumers receive it as a Checker interface.
//
// The struct is exported (uppercase H) so main.go can create one with NewHTTPChecker().
// But its field (client) is unexported — callers can't mess with the internal client.
type HTTPChecker struct {
	client *http.Client // connection pool — reused across ALL calls to Check()
}

// NewHTTPChecker is the constructor for HTTPChecker.
//
// WHY A CONSTRUCTOR FUNCTION instead of just `HTTPChecker{}`?
//   1. We set up the http.Client with non-default behaviour (redirect policy).
//   2. Zero-value of http.Client would follow all redirects and have no timeout —
//      both wrong for a monitoring tool.
//   3. A constructor makes it impossible to create a broken HTTPChecker.
//
// defaultTimeout is the fallback deadline baked into the http.Client itself.
// The context passed to Check() can impose a STRICTER (shorter) deadline.
// Both deadlines are active simultaneously — whichever fires first wins.
//
// Python: requests.Session() with timeout parameter
// Node.js: axios.create({ timeout: 5000 }) or AbortController
func NewHTTPChecker(defaultTimeout time.Duration) *HTTPChecker {
	return &HTTPChecker{
		client: &http.Client{
			// Timeout is the total time limit for the WHOLE request lifecycle:
			// DNS lookup + TCP connect + TLS handshake + request + response body.
			// If this fires, Do() returns a context.DeadlineExceeded-flavoured error.
			Timeout: defaultTimeout,

			// CheckRedirect controls what happens when the server returns 3xx.
			// By default http.Client follows up to 10 redirects.
			// We STOP at the first redirect and return that response directly.
			//
			// WHY? We're monitoring if a specific URL is alive. If https://foo.com
			// redirects to https://www.foo.com, the original URL is technically a 301 —
			// worth recording. Following the redirect silently would hide that.
			//
			// http.ErrUseLastResponse is Go's magic sentinel: "stop here, use this response".
			// The error is caught internally by http.Client and NOT returned to callers.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Check performs one HTTP GET to url, measures latency, and returns a Result.
// This method makes HTTPChecker satisfy the Checker interface.
//
// METHOD RECEIVER: (h *HTTPChecker)
// "h" is the receiver — like "self" in Python or "this" in Node.js.
// We use a POINTER receiver (*HTTPChecker) because:
//   1. http.Client contains a mutex (sync.Mutex) internally — copying it is a bug.
//   2. Pointer receivers are consistent with how we created the struct (NewHTTPChecker
//      returns *HTTPChecker, not HTTPChecker).
//
// Python:  def check(self, ctx, url: str) -> Result:
// Node.js: check(signal: AbortSignal, url: string): Result
func (h *HTTPChecker) Check(ctx context.Context, url string) Result {
	// ── STEP 1: Start the clock ───────────────────────────────────────────────
	// Record time BEFORE creating the request — we want to include connection
	// setup time in the latency measurement, not just server response time.
	//
	// time.Now() returns a time.Time snapshot.
	// time.Since(start) later gives us the elapsed duration.
	//
	// Python: start = time.perf_counter()  →  latency = time.perf_counter() - start
	// Node.js: const start = performance.now()  →  performance.now() - start
	start := time.Now()

	// ── STEP 2: Build the request WITH context ────────────────────────────────
	// http.NewRequestWithContext ties the HTTP request to ctx.
	// If ctx is cancelled (deadline exceeded, SIGTERM, etc.) WHILE the request
	// is in-flight, the underlying TCP connection is immediately aborted.
	//
	// WHY NOT http.Get(url)?
	// http.Get does not accept a context → you can't cancel it mid-flight.
	// In a worker pool with N goroutines, a hanging http.Get holds that goroutine
	// hostage forever. Context cancellation is how Go avoids this.
	//
	// Parameters:
	//   ctx          → cancellation/deadline signal
	//   http.MethodGet → "GET" string constant (prefer constants over raw strings)
	//   url          → the target URL
	//   nil          → request body (GET requests have no body)
	//
	// Python: httpx.get(url, timeout=httpx.Timeout(...))  with async support
	// Node.js: fetch(url, { signal: abortController.signal })
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		// Only happens if url is malformed (e.g., "not a url").
		// This is a programming error, not a network error.
		// We still return a Result (not panic, not raw error) for API consistency.
		return newErrorResult(
			time.Since(start),
			fmt.Errorf("build request for %q: %w", url, err),
		)
	}

	// Set a User-Agent so monitored servers can identify Pulse in their logs.
	// Without a User-Agent, some servers (Cloudflare WAF, nginx defaults) return 403.
	req.Header.Set("User-Agent", "Pulse-Monitor/1.0")

	// ── STEP 3: Execute the request ───────────────────────────────────────────
	// h.client.Do(req) sends the request and returns the response.
	// It blocks until:
	//   a) The response headers are received (not the full body), OR
	//   b) An error occurs (timeout, DNS failure, context cancelled)
	//
	// Two deadlines compete here:
	//   - h.client.Timeout (set in NewHTTPChecker, e.g., 10 seconds)
	//   - req.Context() deadline (set by the caller, e.g., 5 seconds)
	// Whichever deadline fires first cancels the request.
	//
	// Python: response = session.get(url, timeout=5)
	// Node.js: const response = await fetch(url, { signal })
	resp, err := h.client.Do(req)

	// ── STEP 4: Measure latency ───────────────────────────────────────────────
	// Measure IMMEDIATELY after Do() returns — before any processing.
	// time.Since(start) = time.Now().Sub(start) — measures elapsed wall time.
	// .Milliseconds() converts nanoseconds → milliseconds (discards nanosecond fraction).
	latency := time.Since(start)

	if err != nil {
		// Network-level failure: DNS lookup failed, connection refused, timeout.
		// We never got an HTTP response, so StatusCode is 0.
		// Wrap the error with %w so errors.Is() can unwrap through the chain.
		return newErrorResult(latency, fmt.Errorf("execute request: %w", err))
	}

	// ── STEP 5: defer resp.Body.Close() ──────────────────────────────────────
	//
	// ╔══════════════════════════════════════════════════════════════════════╗
	// ║  THIS IS THE MOST IMPORTANT LINE — READ THIS CAREFULLY.             ║
	// ╚══════════════════════════════════════════════════════════════════════╝
	//
	// resp.Body is an io.ReadCloser — a stream backed by the TCP connection.
	// You MUST close it when you're done, for two reasons:
	//
	//   1. CONNECTION POOL: http.Client maintains a pool of keep-alive TCP connections.
	//      When you close the body, the connection is returned to the pool and
	//      reused by the next request (fast — no TCP handshake needed).
	//      If you DON'T close: the connection is NEVER returned. Each Check()
	//      call opens a new TCP connection. Under load you exhaust the OS
	//      file descriptor limit ("too many open files") and the process crashes.
	//
	//   2. MEMORY LEAK: The Body holds a reference to a network buffer.
	//      Not closing it means the GC can't free that memory.
	//
	// WHAT IS defer?
	// defer schedules a function call to run when the ENCLOSING FUNCTION returns.
	// It runs even if:
	//   - We return early (e.g., for a non-2xx check)
	//   - The function panics (Recoverer middleware catches the panic, body still closed)
	//
	// defer is guaranteed — you write it ONCE right after Do() and forget about it.
	// If you used a try/finally approach you'd have to remember to close in every path.
	//
	// RULE: Always write defer resp.Body.Close() as the VERY NEXT LINE after err check.
	//
	// Python: `with requests.Session() as s: r = s.get(url)` — context manager closes body
	//         or: `try: ... finally: r.close()`
	// Node.js: response.body.cancel() if you don't read it — rarely done explicitly
	// Go:      `defer resp.Body.Close()` — idiomatic, guaranteed, one line
	defer resp.Body.Close()

	// ── STEP 6: Drain the body ────────────────────────────────────────────────
	// We don't need the body content (we only care about status + latency).
	// But we MUST fully read the body before closing it.
	//
	// WHY? HTTP/1.1 connection reuse requires the response body to be fully consumed.
	// If you close without reading, the connection can't be reused (the server might
	// still be writing data into it). io.Copy(io.Discard, resp.Body) reads everything
	// and throws it away — cheap, but ensures the connection is cleanly reusable.
	//
	// io.Discard is a write-only sink (like /dev/null).
	// We ignore the error: if reading fails here, the connection will be discarded
	// anyway, which is acceptable.
	//
	// Python: response.content  or  response.iter_content() — reads and discards
	// Node.js: await response.arrayBuffer() if you don't need the data
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	// ── STEP 7: Build and return the Result ──────────────────────────────────
	return Result{
		StatusCode: resp.StatusCode,           // e.g., 200, 404, 503
		LatencyMs:  latency.Milliseconds(),    // e.g., 45 (ms)
		Up:         IsUp(resp.StatusCode),     // true only for 2xx
		Err:        nil,                       // we got a response — no transport error
	}
}
