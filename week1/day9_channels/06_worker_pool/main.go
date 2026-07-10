package main

import (
	"fmt"
	"sync"
	"time"
)

// Worker pool: fixed number of goroutines share a job queue.
// No matter how many jobs arrive (10 or 10 million), only N goroutines run.
// This bounds memory and CPU usage — machines don't crash.
//
// Production use cases:
//   - Resize 50,000 images with 10 workers
//   - Send bulk emails with 20 workers
//   - Process CSV rows with 5 workers

type Job struct {
	ID    int
	Input string
}

type Result struct {
	JobID  int
	Output string
}

func worker(id int, jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobs { // exits when jobs channel is closed and drained
		// simulate work (e.g. calling an external API)
		time.Sleep(10 * time.Millisecond)
		results <- Result{
			JobID:  job.ID,
			Output: fmt.Sprintf("[worker-%d] processed: %s", id, job.Input),
		}
	}
}

func main() {
	const numWorkers = 3
	const numJobs = 9

	jobs := make(chan Job, numJobs)
	results := make(chan Result, numJobs)

	var wg sync.WaitGroup

	// Step 1 — spin up the fixed worker pool
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go worker(w, jobs, results, &wg)
	}

	// Step 2 — enqueue all jobs, then close so workers exit their range loop
	for j := 1; j <= numJobs; j++ {
		jobs <- Job{ID: j, Input: fmt.Sprintf("email-%d@example.com", j)}
	}
	close(jobs) // workers see this and exit when queue is empty

	// Step 3 — close results after all workers finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Step 4 — collect all results
	for r := range results {
		fmt.Printf("result job=%d: %s\n", r.JobID, r.Output)
	}

	fmt.Println("all jobs done")

	// --- bonus: semaphore pattern (same idea without explicit pool) ---
	// Limits concurrency to 2 simultaneous goroutines using a buffered channel.
	sem := make(chan struct{}, 2)
	var swg sync.WaitGroup

	for i := 1; i <= 5; i++ {
		swg.Add(1)
		go func(id int) {
			defer swg.Done()
			sem <- struct{}{}        // acquire slot
			defer func() { <-sem }() // release slot when done
			fmt.Printf("semaphore goroutine %d running (active: %d)\n", id, len(sem))
			time.Sleep(20 * time.Millisecond)
		}(i)
	}
	swg.Wait()
	fmt.Println("semaphore demo done")
}
