package main

import "fmt"

// Buffered channel = letterbox / mailbox.
// Sender can drop items without blocking — until the box is full.
// Receiver can pick up items without blocking — until the box is empty.
// Blocks only at the edges: send when full, receive when empty.

func main() {
	// --- demo 1: basic buffered send without blocking ---
	ch := make(chan int, 3) // buffer holds up to 3 ints

	ch <- 10 // no goroutine needed — puts in buffer, returns immediately
	ch <- 20
	ch <- 30
	fmt.Println("sent 3 items without blocking")

	// 4th send would BLOCK — buffer full, nobody reading
	// ch <- 40  // uncomment to see deadlock

	fmt.Println(<-ch) // 10 — FIFO order
	fmt.Println(<-ch) // 20
	fmt.Println(<-ch) // 30

	// --- demo 2: len and cap ---
	letters := make(chan string, 5)
	letters <- "a"
	letters <- "b"
	letters <- "c"
	fmt.Printf("len=%d cap=%d\n", len(letters), cap(letters)) // len=3 cap=5

	// --- demo 3: burst producer + slow consumer ---
	// Producer fires all jobs into a buffered channel without waiting.
	// Consumer works at its own pace.
	jobs := make(chan int, 10)

	// producer — runs in same goroutine, no blocking because buffer is big enough
	for i := 1; i <= 5; i++ {
		jobs <- i
		fmt.Printf("produced job %d\n", i)
	}
	close(jobs)

	// consumer
	for j := range jobs {
		fmt.Printf("consumed job %d\n", j)
	}

	// --- demo 4: buffer as semaphore (limit concurrency) ---
	// A channel of capacity N is a classic semaphore: acquire = send, release = receive.
	sem := make(chan struct{}, 2) // allow at most 2 concurrent "slots"

	acquire := func() { sem <- struct{}{} }
	release := func() { <-sem }

	for i := 1; i <= 4; i++ {
		acquire()
		fmt.Printf("slot acquired by task %d (slots in use: %d)\n", i, len(sem))
		release()
	}
}
