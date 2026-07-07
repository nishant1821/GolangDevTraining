package main

import (
	"fmt"
	"sync"
)

func worker(id int, wg *sync.WaitGroup) {
	defer wg.Done() // always defer — runs even if the goroutine panics
	fmt.Printf("worker %d: starting\n", id)
	// simulate work (no sleep so the race detector stays quiet)
	for i := 0; i < 3; i++ {
		_ = i * id
	}
	fmt.Printf("worker %d: done\n", id)
}

func main() {
	var wg sync.WaitGroup

	for i := 1; i <= 5; i++ {
		wg.Add(1)              // register BEFORE starting the goroutine
		go worker(i, &wg)
	}

	wg.Wait() // blocks until all 5 goroutines call Done
	fmt.Println("all workers finished")
}
