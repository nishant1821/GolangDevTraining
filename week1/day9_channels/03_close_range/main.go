package main

import "fmt"

// Closing a channel is the sender's way of saying "no more data coming."
// Two rules:
//   1. Only the SENDER closes — if receiver closes, sender panics on next send.
//   2. Sending to a closed channel PANICS immediately.
//
// Receiving from a closed channel:
//   - Returns remaining buffered values first.
//   - Then returns zero value + ok=false forever (no panic).

func generate(nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		for _, n := range nums {
			out <- n
		}
		close(out) // sender closes when done
	}()
	return out
}

func main() {
	// --- demo 1: manual receive with ok check ---
	ch := make(chan int, 3)
	ch <- 1
	ch <- 2
	close(ch)

	v, ok := <-ch
	fmt.Printf("v=%d ok=%v\n", v, ok) // v=1 ok=true

	v, ok = <-ch
	fmt.Printf("v=%d ok=%v\n", v, ok) // v=2 ok=true

	v, ok = <-ch
	fmt.Printf("v=%d ok=%v\n", v, ok) // v=0 ok=false — channel drained and closed

	// --- demo 2: range over channel (cleanest pattern) ---
	// range exits automatically when channel is closed and drained.
	for val := range generate(10, 20, 30, 40) {
		fmt.Println("range received:", val)
	}
	fmt.Println("range done — channel was closed by generate()")

	// --- demo 3: nil channel blocks forever (useful in select to disable a case) ---
	var nilCh chan int // nil channel
	select {
	case v := <-nilCh: // never fires — nil receive blocks forever
		fmt.Println("got", v)
	default:
		fmt.Println("nil channel case skipped — default hit")
	}

	// --- demo 4: panic demo (commented out) ---
	// Uncomment to see "send on closed channel" panic:
	// closed := make(chan int)
	// close(closed)
	// closed <- 1
}
