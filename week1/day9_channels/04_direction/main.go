package main

import "fmt"

// Channel direction restricts what a function can do with a channel.
//   chan<- T  — send-only  (arrow points INTO channel)
//   <-chan T  — receive-only (arrow points OUT OF channel)
//
// Go enforces this at compile time — you cannot accidentally receive from
// a send-only channel or send to a receive-only channel.
// Bidirectional chan T auto-converts to either direction when passed.

// produce can only SEND — it cannot accidentally drain the channel.
func produce(out chan<- int, count int) {
	for i := 1; i <= count; i++ {
		out <- i
	}
	close(out) // sender is responsible for close
}

// transform reads from in (receive-only), squares values, sends to out (send-only).
func transform(in <-chan int, out chan<- int) {
	for v := range in {
		out <- v * v
	}
	close(out)
}

// consume can only RECEIVE — it cannot accidentally send or close.
func consume(in <-chan int) {
	for v := range in {
		fmt.Println("consumed:", v)
	}
}

// wrapInDirection shows the conversion: bidirectional → directional.
func sendOnly(ch chan<- string)  { ch <- "hello" }
func recvOnly(ch <-chan string) string { return <-ch }

func main() {
	// --- demo 1: full directed pipeline ---
	raw := make(chan int)
	squared := make(chan int)

	go produce(raw, 5)      // raw is chan int — Go converts to chan<- int automatically
	go transform(raw, squared)
	consume(squared)

	// --- demo 2: bidirectional ↔ directional conversion ---
	ch := make(chan string, 1) // bidirectional
	sendOnly(ch)              // passes as chan<- string
	msg := recvOnly(ch)       // passes as <-chan string
	fmt.Println("got:", msg)

	// --- demo 3: compile-time safety (uncomment to see errors) ---
	// var sendCh chan<- int = make(chan int)
	// _ = <-sendCh  // ERROR: receive from send-only channel

	// var recvCh <-chan int = make(chan int)
	// recvCh <- 1   // ERROR: send to receive-only channel
}
