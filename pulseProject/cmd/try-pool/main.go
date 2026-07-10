// cmd/try-pool/main.go — runnable demo of Scheduler + RunPool wired together.
//
// Run:  go run ./cmd/try-pool
//
// What it does:
//   1. Creates a fakeChecker (no network calls).
//   2. Starts a Scheduler with a 200ms tick and 3 hardcoded "due" monitors.
//   3. Feeds the Scheduler's jobs channel into RunPool (2 workers).
//   4. Prints every Outcome as it arrives.
//   5. After 700ms, cancels the context — watch the full teardown.
//
// This is NOT production code — it's a learning tool to see the pipeline live.
package main

import (
	"context"  // context.WithTimeout — auto-cancel after 700ms
	"fmt"      // fmt.Printf — print outcomes
	"time"     // time.Millisecond

	"github.com/nishantks908/pulse/internal/checker"  // checker.Checker, checker.Result
	"github.com/nishantks908/pulse/internal/domain"   // domain.Monitor
	"github.com/nishantks908/pulse/internal/monitor"  // monitor.Scheduler, monitor.RunPool
)

// ── fakeChecker: no network, instant result ───────────────────────────────────
// Same pattern as pool_test.go — any struct with Check() satisfies Checker.
type fakeChecker struct{}

func (fakeChecker) Check(_ context.Context, url string) checker.Result {
	// Simulate 50ms of "network" time so output isn't instant.
	time.Sleep(50 * time.Millisecond)
	return checker.Result{StatusCode: 200, LatencyMs: 50, Up: true}
}

func main() {
	// ── 1. Context with 700ms timeout ─────────────────────────────────────────
	// After 700ms, ctx is auto-cancelled — tears down the whole pipeline.
	// WithTimeout = WithDeadline(now + 700ms) — same thing, more readable.
	//
	// Python: asyncio.wait_for(coro(), timeout=0.7)
	// Node.js: AbortSignal.timeout(700)
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel() // always release timer even if we return early

	// ── 2. Hardcoded "due" monitors (stand-in for DB query) ───────────────────
	// In Stage 5 this will be: repo.FindDueMonitors(ctx)
	dueMonitors := []domain.Monitor{
		{ID: 1, URL: "https://alpha.example.com"},
		{ID: 2, URL: "https://beta.example.com"},
		{ID: 3, URL: "https://gamma.example.com"},
	}
	due := func() []domain.Monitor { return dueMonitors }

	// ── 3. Start Scheduler ────────────────────────────────────────────────────
	// Tick every 200ms — 700ms context gives us ~3 ticks (t=0ms, t=200ms, t=400ms,
	// t=600ms) before ctx cancels. Each tick enqueues 3 jobs → ~12 jobs total.
	//
	// Scheduler owns the jobs channel. When ctx cancels:
	//   scheduler goroutine exits → defer close(jobs) fires
	jobs := monitor.Scheduler(ctx, 200*time.Millisecond, due)

	// ── 4. Start Pool ─────────────────────────────────────────────────────────
	// 2 workers reading from jobs channel.
	// When jobs closes (from scheduler shutdown):
	//   workers exit → wg.Done() × 2 → wg.Wait() returns → close(out) fires
	outcomes := monitor.RunPool(ctx, fakeChecker{}, 2, jobs)

	// ── 5. Drain outcomes ─────────────────────────────────────────────────────
	// `range outcomes` blocks until close(out) — which happens automatically
	// when the pipeline tears down after ctx cancel.
	fmt.Println("Pipeline started. Outcomes:")
	fmt.Println("─────────────────────────────────────────")

	count := 0
	for o := range outcomes {
		status := "✅ UP"
		if !o.Result.Up {
			status = "❌ DOWN"
		}
		fmt.Printf("%s  monitor=%-2d  url=%-30s  latency=%dms\n",
			status, o.Job.MonitorID, o.Job.URL, o.Result.LatencyMs)
		count++
	}

	// range exits here because close(out) fired — pipeline is fully shut down.
	fmt.Println("─────────────────────────────────────────")
	fmt.Printf("Pipeline stopped cleanly. Total outcomes: %d\n", count)
}
