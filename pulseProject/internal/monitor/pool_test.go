package monitor

// pool_test.go — verifies RunPool with go test -race.
//
// We inject a fakeChecker instead of a real HTTPChecker so that:
//   - Tests run in milliseconds (no network I/O).
//   - Tests are deterministic (no DNS, no server latency).
//   - Tests pass in CI without an outbound internet connection.
//
// Python: unittest.mock.Mock() or a stub class that satisfies the Protocol.
// Node.js: jest.fn() or a simple object literal { check: () => result }.
// Go:      a tiny struct that satisfies the Checker interface — duck typing,
//          no "implements Checker" declaration needed.

import (
	"context"  // context.Background — a never-cancelled context for happy-path tests
	"fmt"      // fmt.Sprintf — build distinct URLs for each job
	"testing"  // testing.T — Go's built-in test harness

	"github.com/nishantks908/pulse/internal/checker" // checker.Checker, checker.Result
)

// ─────────────────────────────────────────────────────────────────────────────
// fakeChecker — test double for checker.Checker
// ─────────────────────────────────────────────────────────────────────────────

// fakeChecker satisfies checker.Checker without making network calls.
// Any type with a Check(context.Context, string) checker.Result method
// automatically satisfies the interface — Go's structural typing.
//
// Python: class FakeChecker: def check(self, ctx, url) -> Result: ...
// Node.js: const fakeChecker = { check: (signal, url) => ({ statusCode: 200, up: true }) }
type fakeChecker struct{}

// Check returns a hardcoded successful Result instantly.
// The blank identifiers _ mean "I receive these parameters but don't use them".
//
// Python: def check(self, ctx, url): return Result(status_code=200, latency_ms=1, up=True)
// Node.js: check(signal, url) { return { statusCode: 200, latencyMs: 1, up: true } }
func (fakeChecker) Check(_ context.Context, _ string) checker.Result {
	return checker.Result{
		StatusCode: 200,
		LatencyMs:  1,
		Up:         true,
		Err:        nil,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestRunPool_20jobs_5workers feeds 20 jobs to a 5-worker pool and asserts
// that every job produces exactly one Outcome with Up=true.
//
// Run with: go test -race ./internal/monitor/...
//
// -race instruments every channel send/receive and memory access to detect
// data races at runtime. "race-clean" means this test produces zero race
// reports under -race — proving the pool uses channels (not shared memory)
// for all communication between goroutines.
func TestRunPool_20jobs_5workers(t *testing.T) {
	const (
		numJobs    = 20 // total work items
		numWorkers = 5  // goroutines in the pool (4 jobs per worker on average)
	)

	ctx := context.Background() // never cancelled — happy-path test

	// ── Build the jobs channel ────────────────────────────────────────────────
	// Buffered at numJobs so the loop below never blocks — we can fill the
	// whole channel before the pool even starts running.
	//
	// Python: queue = asyncio.Queue(maxsize=numJobs); [queue.put_nowait(j) for j in jobs]
	// Node.js: const jobs = new BroadcastChannel(); jobs are queued as messages
	jobs := make(chan Job, numJobs)
	for i := 0; i < numJobs; i++ {
		jobs <- Job{
			MonitorID: uint(i + 1),
			URL:       fmt.Sprintf("https://site%d.example.com", i+1),
		}
	}

	// close(jobs) is what tells workers "no more jobs are coming".
	// Workers' `case job, ok := <-jobs` returns ok=false when the channel is
	// both empty AND closed — this is how `range jobs` knows when to stop too.
	//
	// GOTCHA: if you forget close(jobs), workers wait forever for the next job
	// → wg.Done() is never called → wg.Wait() hangs → close(out) never fires
	// → the test hangs. Always close the producer side of a jobs channel.
	//
	// Python: queue.join() + task_done() — a different mechanism
	// Node.js: readable.push(null) to signal end-of-stream
	close(jobs)

	// ── Start the pool ────────────────────────────────────────────────────────
	// RunPool returns immediately (non-blocking). Workers are already running.
	// outcomes will be closed automatically once all workers finish.
	outcomes := RunPool(ctx, fakeChecker{}, numWorkers, jobs)

	// ── Drain all outcomes ────────────────────────────────────────────────────
	// `range outcomes` blocks until close(out) fires inside RunPool's closer goroutine.
	// This is Go's idiomatic "read everything from a channel until it closes".
	//
	// Python: async for outcome in outcomes_queue: ...
	// Node.js: for await (const outcome of outcomesStream) { ... }
	var got int
	for o := range outcomes {
		// Verify each outcome reports success.
		if !o.Result.Up {
			t.Errorf("job MonitorID=%d URL=%s: expected Up=true, got Up=false (err: %v)",
				o.Job.MonitorID, o.Job.URL, o.Result.Err)
		}
		got++
	}

	// Verify all 20 jobs produced an outcome — none were dropped or duplicated.
	if got != numJobs {
		t.Errorf("expected %d outcomes, got %d", numJobs, got)
	}
}

// TestRunPool_ctxCancel verifies that cancelling ctx stops workers cleanly
// and the outcomes channel is still closed (no leak, no hang).
func TestRunPool_ctxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Unbuffered jobs channel that will never be written to or closed.
	// Workers will block forever on `case job := <-jobs` … unless ctx is cancelled.
	jobs := make(chan Job)

	outcomes := RunPool(ctx, fakeChecker{}, 3, jobs)

	// Cancel the context — all 3 workers should hit `case <-ctx.Done()` and return.
	cancel()

	// Drain outcomes (there should be zero — no jobs were sent).
	// If workers leaked (goroutine leak), this range would hang indefinitely
	// because close(out) would never fire.
	var got int
	for range outcomes {
		got++
	}

	// No jobs were sent, so no outcomes should appear.
	if got != 0 {
		t.Errorf("expected 0 outcomes after cancel, got %d", got)
	}
}
