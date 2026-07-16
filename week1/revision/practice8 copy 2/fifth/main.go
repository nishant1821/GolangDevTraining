package main

import (
	"fmt"
	"time"
)

func slowFetch(ch chan<- string) {
	time.Sleep(3 * time.Second) // 3s lagta hai (jaan-bujhke slow)
	ch <- "data mila"
}

func main() {
	ch := make(chan string)
	go slowFetch(ch)

	// YOUR TURN: select se — result aaye to print, warna 2s baad timeout
	select {
	case result := <-ch:
		fmt.Println(result)

	case <-time.After(2 * time.Second):
		fmt.Println("terminate")

	}
	// slowFetch 3s leta hai, timeout 2s — to "timeout" chalega
}
