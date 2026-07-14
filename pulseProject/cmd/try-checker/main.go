// cmd/try-checker/main.go — throwaway demo. Run once to see the checker in action.
// This file is NOT part of the production server. Delete it after Stage 6.
//
// Run:
//   go run ./cmd/try-checker
//
// Expected output (approx):
//   ✅ UP   https://google.com              status=301  latency=120ms
//   ❌ DOWN https://httpbin.org/status/503  status=503  latency=210ms
//   ❌ DOWN https://httpbin.org/delay/10    status=0    latency=4001ms  | timeout
//   ❌ DOWN https://this.invalid            status=0    latency=5ms     | DNS error

package main

import (
	"context" // context.WithTimeout — gives entire batch a time limit
	"fmt"     // fmt.Printf — formatted output to stdout
	"os"      // os.Stderr — print warnings without mixing with normal output
	"time"    // time.Second, time.Duration

	"github.com/nishantks908/pulse/internal/checker" // our Checker interface + HTTPChecker
)

func main() {
	// ── Create the checker ───────────────────────────────────────────────────
	// NewHTTPChecker returns *HTTPChecker — satisfies the Checker interface.
	// We assign it to a checker.Checker variable to prove it fits the interface.
	//
	// defaultTimeout = 5 seconds per individual request.
	// The context below adds a SECOND, shorter overall deadline.
	var c checker.Checker = checker.NewHTTPChecker(5 * time.Second)

	// ── URLs to probe ────────────────────────────────────────────────────────
	// A mix of cases: success, server-side error, timeout, DNS failure.
	urls := []string{
		"https://example.com",              // 200 — UP (IANA stable test domain)
		"https://google.com",               // 301 redirect — DOWN (we stop at redirects)
		"https://httpstat.us/503",          // always returns 503 — DOWN (reachable but unhealthy)
		"https://httpstat.us/200?sleep=10000", // artificial 10s delay — cut by our 4s context
		"https://this-domain-does-not-exist.xyz", // DNS failure — DOWN
	}

	// ── Context with overall deadline ────────────────────────────────────────
	// The WHOLE batch has 4 seconds.
	// Each individual check also has the 5-second client timeout.
	// The context fires FIRST (4s < 5s) for any check still running at 4s.
	//
	// context.Background() → root context (no deadline, no cancel)
	// context.WithTimeout  → child context that cancels after 4 seconds
	// cancel()             → explicitly release context resources (via defer)
	//
	// GOTCHA: always defer cancel() — if you forget, the timer goroutine inside
	// context.WithTimeout leaks until the deadline fires naturally.
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel() // will run when main() returns

	fmt.Println("Pulse Checker — probing URLs...")

	for _, url := range urls {
		// c.Check() blocks until the response arrives or context/timeout fires.
		// It ALWAYS returns a Result — never panics, never returns nil.
		result := c.Check(ctx, url)

		// ── Format output ─────────────────────────────────────────────────────
		upLabel := "✅ UP  "
		if !result.Up {
			upLabel = "❌ DOWN"
		}

		errStr := ""
		if result.Err != nil {
			// Trim the error to one line — full wrapping chain can be verbose
			errStr = fmt.Sprintf(" | %v", result.Err)
		}

		fmt.Printf("%s  %-45s  status=%-3d  latency=%dms%s\n",
			upLabel,
			url,
			result.StatusCode,
			result.LatencyMs,
			errStr,
		)
	}

	// ── Context status ────────────────────────────────────────────────────────
	// If the overall 4-second deadline fired mid-loop, report it.
	// ctx.Err() returns nil if the context is still alive, or the reason it ended.
	if ctx.Err() != nil {
		fmt.Fprintf(os.Stderr, "\n⚠️  Overall context ended: %v\n", ctx.Err())
	}
}
