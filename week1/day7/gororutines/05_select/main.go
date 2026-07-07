package main

import (
	"fmt"
	"time"
)

func fastProducer(ch chan string) {
	time.Sleep(50 * time.Millisecond)
	ch <- "fast result"
}

func slowProducer(ch chan string) {
	time.Sleep(500 * time.Millisecond)
	ch <- "slow result"
}

func main() {
	fast := make(chan string, 1)
	slow := make(chan string, 1)

	go fastProducer(fast)
	go slowProducer(slow)

	// select picks whichever channel is ready first
	for i := 0; i < 2; i++ {
		select {
		case msg := <-fast:
			fmt.Println("received from fast:", msg)
		case msg := <-slow:
			fmt.Println("received from slow:", msg)
		}
	}

	// select with default — non-blocking check
	ch := make(chan int, 1)
	select {
	case v := <-ch:
		fmt.Println("got:", v)
	default:
		fmt.Println("channel empty — continuing without blocking")
	}

	// Timeout pattern
	result := make(chan string, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		result <- "computed"
	}()

	select {
	case r := <-result:
		fmt.Println("got result:", r)
	case <-time.After(1 * time.Second):
		fmt.Println("timed out")
	}
}
