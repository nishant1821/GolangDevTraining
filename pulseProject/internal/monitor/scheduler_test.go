package monitor

// scheduler_test.go — verifies Scheduler with a fast tick and a fake `due` func.
//
// No DB, no real HTTP. Two things tested:
//   1. Happy path: every due monitor becomes a Job in the channel.
//   2. Cancel: ctx cancel stops the scheduler and closes the jobs channel cleanly.

import (
	"context"   // context.WithCancel, context.WithTimeout
	"testing"   // testing.T
	"time"      // time.Millisecond — fast tick for tests

	"github.com/nishantks908/pulse/internal/domain" // domain.Monitor
)

// TestScheduler_EnqueuesJobs verifies that every monitor returned by `due`
// appears as a Job in the jobs channel.
func TestScheduler_EnqueuesJobs(t *testing.T) {
	// two monitors that are always "due"
	monitors := []domain.Monitor{
		{ID: 1, URL: "https://site1.example.com"},
		{ID: 2, URL: "https://site2.example.com"},
	}

	// `due` is a closure — pretends to be a DB query.
	// Returns the same two monitors on every call.
	//
	// Python: due = lambda: monitors
	// Node.js: const due = () => monitors
	due := func() []domain.Monitor { return monitors }

	// Very short tick so the test doesn't wait around.
	tick := 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // always release resources

	jobs := Scheduler(ctx, tick, due)

	// Collect the first 2 jobs (one tick worth).
	seen := map[uint]bool{}
	for i := 0; i < len(monitors); i++ {
		select {
		case job := <-jobs:
			seen[job.MonitorID] = true
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for jobs")
		}
	}

	// Both monitors should have been enqueued.
	for _, m := range monitors {
		if !seen[m.ID] {
			t.Errorf("MonitorID %d was not enqueued", m.ID)
		}
	}
}

// TestScheduler_CancelClosesJobs verifies that cancelling ctx causes the
// jobs channel to be closed — which is what lets RunPool workers exit.
func TestScheduler_CancelClosesJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// `due` returns nothing — we just want to test shutdown.
	jobs := Scheduler(ctx, 10*time.Millisecond, func() []domain.Monitor { return nil })

	// Cancel after a short delay — let at least one tick fire first.
	time.Sleep(30 * time.Millisecond)
	cancel()

	// After cancel, drain any buffered jobs and wait for close.
	// If the channel is never closed, this loop hangs → test times out.
	timeout := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-jobs:
			if !ok {
				return // channel closed cleanly ✅
			}
		case <-timeout:
			t.Fatal("jobs channel was never closed after ctx cancel")
		}
	}
}
