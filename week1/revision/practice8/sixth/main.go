package main

import (
	"fmt"
	"sync"
)

func worker(id int, jobs <-chan int, results chan<- int, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range jobs { // jobs channel se uthaao (jab tak close na ho)
		fmt.Printf("Worker %d ne job %d liya\n", id, job)
		// YOUR TURN: results me job*2 bhejo (kaam = double karna)
		results <- job * 2

	}
}

func main() {
	jobs := make(chan int, 10)
	results := make(chan int, 10)
	var wg sync.WaitGroup

	// 5 workers launch karo
	for w := 1; w <= 5; w++ {
		wg.Add(1)
		go worker(w, jobs, results, &wg)
	}

	// YOUR TURN: 10 jobs daalo channel me (1 se 10)
	for j := 1; j <= 10; j++ {
		jobs <- j

	}
	close(jobs) // jobs khatam — workers ka range ruk jaayega

	// alag goroutine me wait karo, phir results close karo
	go func() {
		wg.Wait()
		close(results)
	}()

	// YOUR TURN: results range karke sab print karo
	for r := range results {
		fmt.Println("Result:", r)
	}
}
