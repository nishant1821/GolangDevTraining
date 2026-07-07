package main

import (
	"fmt"
	"sync"
)

func workerPool(id int, jobs <-chan int, results chan<- int, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobs { // exits when jobs channel is closed
		fmt.Printf("worker %d processing job %d\n", id, job)
		results <- job * job // send square of the job number
	}
}

func main() {
	const numWorkers = 3
	const numJobs = 9

	jobs := make(chan int, numJobs)
	results := make(chan int, numJobs)

	var wg sync.WaitGroup

	// Start the worker pool
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go workerPool(w, jobs, results, &wg)
	}

	// Send all jobs, then close the channel so workers exit their range loop
	for j := 1; j <= numJobs; j++ {
		jobs <- j
	}
	close(jobs)

	// Wait for all workers to finish, then close results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for r := range results {
		fmt.Println("result:", r)
	}
}
