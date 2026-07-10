package monitor

// scheduler.go — Stage 4.
//
// Scheduler's one job: on every tick, ask "which monitors are due?" and
// push a Job for each into the jobs channel.
//
// It owns the jobs channel lifecycle:
//   - creates it
//   - writes into it
//   - closes it when done
//
// This is the PRODUCER side of the Scheduler → Pool pipeline:
//
//   Scheduler ──► [jobs chan] ──► RunPool ──► [out chan] ──► Result Handler
//
// WHY does Scheduler own close(jobs)?
//   Only the writer should close a channel. Scheduler is the only goroutine
//   that sends to jobs. So it — and only it — calls close(jobs).
//   When jobs closes, workers' `case job, ok := <-jobs` returns ok=false → workers exit.
//   Workers exit → wg.Done() × N → closer goroutine fires → close(out) → caller done.
//   One cancel() tears the whole pipeline down in sequence.

import (
	"context"  // context.Context — cancellation signal from main.go
	"time"     // time.Ticker, time.Duration — the heartbeat

	"github.com/nishantks908/pulse/internal/domain" // domain.Monitor
)

// Scheduler ticks on `tick` interval, calls `due()` to get monitors that need
// checking, and enqueues one Job per monitor into the returned channel.
//
// The returned channel is closed when ctx is cancelled — that close signal
// propagates through the entire pipeline automatically (see teardown trace below).
//
// Parameters
//   ctx   — pool-wide context; cancel() → scheduler stops → jobs closed → pool stops.
//   tick  — how often to check for due monitors (e.g. 10*time.Second).
//   due   — a function that returns monitors whose next-check time has passed.
//           In production: a repository DB query.
//           In tests:      a slice returned by a closure.
//
// Return
//   <-chan Job — read-only stream for RunPool to consume.
//
// Python asyncio analog:
//   async def scheduler(ctx, tick, due):
//       while not ctx.done():
//           await asyncio.sleep(tick)
//           for m in due():
//               await jobs_queue.put(Job(m.id, m.url))
//
// Node.js analog:
//   const id = setInterval(() => { due().forEach(m => jobs.push(m)) }, tick)
//   signal.addEventListener('abort', () => { clearInterval(id); jobs.end() })
func Scheduler(ctx context.Context, tick time.Duration, due func() []domain.Monitor) <-chan Job {

	// Buffered at 100: gives the scheduler room to burst-enqueue multiple monitors
	// in one tick without blocking, even if the pool is momentarily busy.
	//
	// Python: asyncio.Queue(maxsize=100)
	// Node.js: stream with highWaterMark: 100
	jobs := make(chan Job, 100)

	// The scheduler runs entirely inside a single goroutine.
	// We return `jobs` immediately (non-blocking) and let this goroutine run in background.
	//
	// Python: asyncio.create_task(scheduler_coro())
	// Node.js: setInterval is already non-blocking
	go func() {
		// defer close(jobs) — MOST IMPORTANT LINE IN THIS FILE.
		//
		// When this goroutine returns (because ctx was cancelled), defer fires
		// and closes the jobs channel. That close travels through the pipeline:
		//
		//   close(jobs)
		//     → workers' `case job, ok := <-jobs` gets ok=false → workers return
		//     → each worker calls defer wg.Done()
		//     → wg counter reaches 0 → wg.Wait() returns in closer goroutine
		//     → close(out) fires
		//     → caller's `range out` loop exits
		//
		// One cancel() → whole pipeline torn down cleanly, no goroutine leaks.
		//
		// WHY defer and not explicit close() at the end?
		// defer guarantees the close even if we add an early-return path later.
		// Explicit close() at the bottom would be missed if we ever add a
		// `if err != nil { return }` above it.
		defer close(jobs)

		// time.NewTicker creates a ticker that sends the current time on its
		// channel every `tick` duration.
		//
		// IMPORTANT: always defer ticker.Stop() to release the internal timer goroutine.
		// Without Stop(), the ticker keeps firing after this goroutine exits — a leak.
		//
		// Python: asyncio.sleep(tick) in a loop — no explicit cleanup needed
		// Node.js: setInterval → clearInterval in cleanup; same concept as Stop()
		ticker := time.NewTicker(tick)
		defer ticker.Stop() // releases the timer goroutine when scheduler exits

		for {
			select {

			// ── Tick: check for due monitors ─────────────────────────────────
			// ticker.C is a channel that receives the current time every `tick`.
			// We don't use the time value (blank identifier _) — we only care
			// that a tick happened.
			//
			// Python: await asyncio.sleep(tick)  → no explicit channel
			// Node.js: setInterval callback fires → no explicit channel
			// Go:      ticker.C fires             → select case unblocks
			case <-ticker.C:
				// Call the injected `due` function to get monitors that need checking.
				// In production this hits the DB:
				//   SELECT * FROM monitors WHERE active=true AND next_check_at <= NOW()
				// In tests it returns a hardcoded slice — no DB needed.
				monitors := due()

				for _, m := range monitors {
					// Build a Job from the domain.Monitor.
					// Only MonitorID and URL are needed by the checker — the pool
					// doesn't need to know intervals, timeouts, or ownership.
					job := Job{
						MonitorID: m.ID,
						URL:       m.URL,
					}

					// Guard the send with ctx.Done().
					//
					// WHY? If the jobs buffer is full AND ctx is cancelled,
					// `jobs <- job` would block forever — goroutine leak.
					// The ctx.Done() arm lets us exit cleanly instead.
					//
					// This is the same pattern as in pool.go workers:
					// EVERY channel send that could block needs a ctx.Done() guard.
					select {
					case jobs <- job:
						// Sent successfully — continue to next monitor.
					case <-ctx.Done():
						// Context cancelled mid-enqueue.
						// Return immediately; defer close(jobs) will fire.
						return
					}
				}

			// ── Shutdown: ctx cancelled ───────────────────────────────────────
			// This case fires when:
			//   - main.go receives SIGTERM and calls cancel()
			//   - a test calls cancel() directly
			//   - parent context times out
			//
			// We return here; defer close(jobs) and defer ticker.Stop() both fire.
			//
			// Python: ctx.done() check at top of while loop
			// Node.js: signal.aborted check; or AbortSignal 'abort' event
			case <-ctx.Done():
				return
			}
		}
	}()

	// Return immediately — the goroutine runs independently.
	// Caller can start feeding `jobs` to RunPool right away.
	return jobs
}
