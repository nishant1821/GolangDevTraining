package monitor

// pool.go — bounded worker pool (Stage 3).
//
// PATTERN: fan-out / fan-in
//
//   Scheduler ──► jobs chan ──►  worker 0 ──┐
//                           ──►  worker 1 ──┤──► out chan ──► caller
//                           ──►  worker 2 ──┘
//
//   Fan-out: ONE jobs channel, N workers reading from it.
//            Go channels guarantee each Job is received by exactly one goroutine.
//   Fan-in:  N workers, ONE out channel they all write into.
//            The caller reads a single, merged stream of Outcomes.
//
// Python asyncio analogy:
//   queue = asyncio.Queue()
//   results = asyncio.Queue()
//   tasks = [asyncio.create_task(worker(queue, results)) for _ in range(N)]
//   await asyncio.gather(*tasks)
//
// Node.js analogy: a worker_threads pool where each thread reads from a
// shared BroadcastChannel and posts results back to the main thread.

import (
	"context" // context.Context — carries cancellation and deadlines
	"sync"    // sync.WaitGroup — "wait until N goroutines finish"

	"github.com/nishantks908/pulse/internal/checker" // checker.Checker interface, checker.Result
)

// ─────────────────────────────────────────────────────────────────────────────
// Job & Outcome — the channel element types
// ─────────────────────────────────────────────────────────────────────────────

// Job is the unit of work produced by the scheduler and consumed by a worker.
// Small struct — passed BY VALUE through the channel (copied, not referenced).
//
// Python: TypedDict or dataclass  — JobItem(monitor_id=5, url="https://…")
// Node.js: plain object          — { monitorId: 5, url: "https://…" }
// Go:      value struct          — copied into the channel's internal buffer
type Job struct {
	MonitorID       uint   // PK of the monitors row — persisted with the result
	URL             string // the endpoint to probe, e.g. "https://api.example.com/ping"
	IntervalSeconds int    // check interval — service uses this to set NextCheckAt after recording
}

// Outcome is produced by a worker and consumed by the caller of RunPool.
// Pairing Job with Result means the consumer never needs a lookup table.
//
// Python: dataclass  Outcome(job=job, result=result)
// Node.js: { job, result }
type Outcome struct {
	Job    Job
	Result checker.Result // status code, latency, up/down, err
}

// ─────────────────────────────────────────────────────────────────────────────
// RunPool — the public API
// ─────────────────────────────────────────────────────────────────────────────

// RunPool starts `workers` goroutines that read Jobs, call chk.Check, and
// send Outcomes. It returns a receive-only channel that is closed once every
// worker has exited — the caller's `range outcomes` terminates automatically.
//
// Parameters
//   ctx     — pool-wide context; cancelling it shuts every worker down cleanly.
//   chk     — the Checker to use (real HTTPChecker in prod, fakeChecker in tests).
//   workers — how many goroutines to run concurrently (e.g. 10).
//   jobs    — read-only stream of Jobs from the scheduler.
//
// Return
//   <-chan Outcome — merged stream of results; closed when all workers exit.
//
// Python asyncio analog:
//   async def run_pool(ctx, checker, workers, jobs_queue):
//       tasks = [asyncio.create_task(_worker(ctx, checker, jobs_queue, out_queue))
//                for _ in range(workers)]
//       await asyncio.gather(*tasks)
//
// Node.js analog:
//   const pool = new Piscina({ ... })   (worker_threads pool)
func RunPool(
	ctx context.Context,
	chk checker.Checker,
	workers int,
	jobs <-chan Job,
) <-chan Outcome {

	// Buffered at `workers`: each worker can deposit one Outcome without
	// blocking immediately — reduces contention when multiple workers finish
	// near-simultaneously.
	//
	// Python asyncio: asyncio.Queue(maxsize=workers)
	// Node.js:        no direct equivalent; roughly a highWaterMark on a stream
	out := make(chan Outcome, workers)

	// WaitGroup tracks how many workers are still alive.
	// wg.Add must be called BEFORE launching goroutines — if a goroutine
	// calls wg.Done() before a concurrent wg.Add runs, the count goes
	// negative and Go panics.
	//
	// Python: asyncio.gather manages the task count for you.
	// Node.js: Promise.all(workerPromises)
	// Go:      sync.WaitGroup — explicit, manual, zero external dependency
	var wg sync.WaitGroup
	wg.Add(workers) // counter = workers; decremented by each wg.Done()

	// ── FAN-OUT: start N worker goroutines ───────────────────────────────────
	for i := 0; i < workers; i++ {
		// "go func() { … }()" — launch an anonymous goroutine.
		// Goroutines are cheap: ~2 KB initial stack vs ~1 MB for an OS thread.
		// Go's runtime multiplexes goroutines across real OS threads (GOMAXPROCS).
		//
		// Python: asyncio.create_task(coroutine())
		// Node.js: no direct one — JS is single-threaded; I/O concurrency comes
		//          from the event loop, not goroutines.
		go func() {
			// defer guarantees wg.Done() runs when this goroutine's function
			// returns — even on early return due to ctx cancellation.
			// If we forgot this, wg.Wait() would hang forever on a dead goroutine.
			//
			// Python: asyncio.gather handles task completion automatically.
			// Node.js: each promise in Promise.all must settle; the goroutine
			//          equivalent must call wg.Done() in every exit path.
			defer wg.Done()

			// We use an explicit select loop instead of `for job := range jobs`.
			//
			// WHY NOT `range jobs`?
			// `range` only stops when the jobs channel is CLOSED.
			// If the context is cancelled (SIGTERM) while a worker is WAITING
			// for the next job, `range` keeps blocking — the goroutine leaks.
			// The ctx.Done() arm in our select breaks that block.
			//
			// GOROUTINE LEAK without ctx.Done():
			//   worker blocked on `for job := range jobs` ─── ctx cancelled ───►
			//   nobody sends to jobs, nobody closes it ──────────────────────────►
			//   worker sits in memory forever, wg.Wait() never returns,
			//   close(out) never fires, caller's `range out` hangs.
			for {
				select {

				// ── Receive the next job ──────────────────────────────────────
				case job, ok := <-jobs:
					if !ok {
						// `ok == false` means the jobs channel was closed —
						// the scheduler has no more work to send.
						// Return here; defer wg.Done() will fire automatically.
						//
						// `range jobs` does this implicitly; we do it explicitly
						// because we added the ctx.Done() case alongside.
						return
					}

					// Call the checker — real HTTP probe in production.
					// ctx is passed through so the probe is cancelled if the
					// pool shuts down mid-flight (e.g., HTTPChecker aborts the TCP call).
					//
					// Python: result = await checker.check(ctx, job.url)
					// Node.js: const result = await checker.check(signal, job.url)
					result := chk.Check(ctx, job.URL)

					// ── FAN-IN: send Outcome to the shared output channel ──────
					// Another select guards the send — if the consumer stopped
					// reading (e.g., it returned early after a fatal error),
					// `out <- …` would block forever. ctx.Done() rescues us.
					select {
					case out <- Outcome{Job: job, Result: result}:
						// Sent successfully — loop back for the next job.
					case <-ctx.Done():
						// Caller or process shut down — discard this outcome and exit.
						return
					}

				// ── Context cancelled while waiting for the next job ──────────
				case <-ctx.Done():
					// This is the arm that prevents the goroutine leak described above.
					// ctx.Done() is a closed (or soon-to-be-closed) channel; reading from
					// a closed channel returns immediately — unblocking the select.
					//
					// Python: task.cancel() raises CancelledError in `async for job in queue`
					// Node.js: AbortSignal fires, breaks out of the async iterator
					return
				}
			}
		}()
	}

	// ── CLOSER GOROUTINE ──────────────────────────────────────────────────────
	// We CANNOT call wg.Wait() here in RunPool's body, because:
	//
	//   Problem 1 — deadlock if called before returning `out`:
	//     RunPool would block at wg.Wait(); the caller never gets `out`;
	//     nobody reads from `out`; workers block on `out <- …`;
	//     wg.Done() is never called; wg.Wait() never returns. Deadlock.
	//
	//   Problem 2 — close before all writers finish:
	//     If we returned `out` first and then called wg.Wait() + close(out) in
	//     RunPool's body sequentially, that's fine for a single goroutine — but
	//     it's still a blocking call that ties up the current goroutine.
	//     The idiomatic Go solution is always a dedicated closer goroutine.
	//
	// RULE: only ONE goroutine may call close(ch), and it must call close ONLY
	// AFTER it knows no other goroutine will write to ch again.
	// wg.Wait() gives exactly that guarantee.
	//
	// Python: asyncio.gather(*tasks) followed by results_queue.join() achieves the same.
	// Node.js: Promise.all(workerPromises).then(() => stream.end())
	go func() {
		wg.Wait()  // block until the last worker calls wg.Done()
		close(out) // signal to the caller's `range out` loop: stream is done
	}()

	// Return out IMMEDIATELY — the caller can start reading right away.
	// Workers are already running; the closer goroutine will shut the channel
	// down when they finish. No deadlock possible.
	return out
}
