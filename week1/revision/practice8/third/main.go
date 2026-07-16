package main

import "fmt"

// YOUR TURN: send-only channel leta hai, usme 1 se 5 bhejta hai
func sender(out chan<- int) {
	for i := 1; i <= 5; i++ {
		out <- i
	}
	close(out) // bhejna khatam — channel band (agla topic)
}

// YOUR TURN: receive-only channel leta hai, sab print karta hai
func receiver(in <-chan int) {
	for v := range in { // range channel pe — sab tak ghumega
		fmt.Println(v)
	}
}

func main() {
	ch := make(chan int)
	go sender(ch)
	receiver(ch)
}
