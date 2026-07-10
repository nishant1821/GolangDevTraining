package main

import (
	"fmt"
	"time"
)

// select listens on multiple channels simultaneously.
// Whichever case is ready first executes.
// If multiple are ready at the same time, Go picks one at random (fairness).
// default makes select non-blocking — runs immediately if no case is ready.

func slowAPI() <-chan string {
	ch := make(chan string, 1)
	go func() {
		time.Sleep(300 * time.Millisecond)
		ch <- "slow API response"
	}()
	return ch
}

func fastAPI() <-chan string {
	ch := make(chan string, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		ch <- "fast API response"
	}()
	return ch
}

func fanIn(ch1, ch2 <-chan string) <-chan string {
	merged := make(chan string, 2)
	go func() {
		for {
			select {
			case v := <-ch1:
				merged <- v
			case v := <-ch2:
				merged <- v
			}
		}
	}()
	return merged
}

func main() {
	// --- demo 1: first-ready wins ---
	fast := fastAPI()
	slow := slowAPI()

	fmt.Println("waiting for whichever API responds first…")
	select {
	case r := <-fast:
		fmt.Println("winner:", r)
	case r := <-slow:
		fmt.Println("winner:", r)
	}

	// --- demo 2: default (non-blocking) ---
	ch := make(chan int, 1)
	select {
	case v := <-ch:
		fmt.Println("got:", v)
	default:
		fmt.Println("channel empty — default ran, no blocking")
	}

	// --- demo 3: timeout pattern (production bread-and-butter) ---
	// "If the service doesn't respond in 200ms, fail fast."
	result := make(chan string, 1)
	go func() {
		time.Sleep(500 * time.Millisecond) // simulates a slow/hung service
		result <- "too late"
	}()

	select {
	case r := <-result:
		fmt.Println("got result:", r)
	case <-time.After(200 * time.Millisecond):
		fmt.Println("timeout — service too slow, giving up")
	}

	// --- demo 4: done / quit signal ---
	// A goroutine keeps sending until it receives a quit signal.
	numbers := make(chan int)
	quit := make(chan struct{})

	go func() {
		for i := 0; ; i++ {
			select {
			case numbers <- i: // send next number
			case <-quit: // quit signal received
				fmt.Println("generator: stopping")
				return
			}
		}
	}()

	for i := 0; i < 5; i++ {
		fmt.Println("received:", <-numbers)
	}
	quit <- struct{}{} // tell generator to stop
	time.Sleep(10 * time.Millisecond)

	// --- demo 5: fan-in (merge two streams into one) ---
	a := make(chan string, 3)
	b := make(chan string, 3)
	a <- "a1"
	a <- "a2"
	b <- "b1"

	merged := fanIn(a, b)
	for i := 0; i < 3; i++ {
		fmt.Println("fan-in:", <-merged)
	}
}
