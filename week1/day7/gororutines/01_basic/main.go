package main

import (
	"fmt"
	"time"
)

func doWork() {
	fmt.Println("doWork: started")
	time.Sleep(100 * time.Millisecond)
	fmt.Println("doWork: finished")
}

func main() {
	fmt.Println("main: before goroutines")

	go doWork() // starts concurrently; main continues immediately

	go func() { // anonymous goroutine
		fmt.Println("anonymous goroutine: running")
	}()

	// Without this sleep, main exits before the goroutines finish printing.
	// In real code you'd use sync.WaitGroup (see 02_waitgroup.go).
	time.Sleep(300 * time.Millisecond)

	fmt.Println("main: done")
}
