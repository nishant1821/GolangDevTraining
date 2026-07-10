package main

import (
	"fmt"
	"time"
)

// Unbuffered channel = hand-to-hand handoff.
// Sender blocks until receiver is ready, and vice versa.
// This synchronises two goroutines at the exact moment of exchange.

func pinger(ch chan string) {
	fmt.Println("pinger: about to send — will BLOCK until receiver is ready")
	ch <- "ping" // blocks here until main receives
	fmt.Println("pinger: send complete — receiver took it")
}

func main() {
	ch := make(chan int) // unbuffered: no internal queue, cap == 0

	// --- demo 1: basic handoff ---
	go func() {
		fmt.Println("goroutine: sending 42 — blocking until main receives")
		ch <- 42 // goroutine blocks here
		fmt.Println("goroutine: send unblocked — main received")
	}()

	time.Sleep(50 * time.Millisecond) // let goroutine reach the send first
	fmt.Println("main: about to receive")
	v := <-ch // main unblocks the goroutine by receiving
	fmt.Println("main: received", v)

	// --- demo 2: sync two goroutines via handoff ---
	ready := make(chan struct{}) // struct{} costs 0 bytes — common "signal" idiom

	go func() {
		fmt.Println("worker: doing setup…")
		time.Sleep(80 * time.Millisecond)
		ready <- struct{}{} // signal that setup is done
	}()

	<-ready // main waits until worker signals
	fmt.Println("main: worker is ready, proceeding")

	// --- demo 3: pinger ---
	strCh := make(chan string)
	go pinger(strCh)
	time.Sleep(30 * time.Millisecond) // let pinger reach its send
	msg := <-strCh
	fmt.Println("main: got", msg)
}
