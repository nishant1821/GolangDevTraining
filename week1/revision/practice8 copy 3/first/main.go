package main

import "fmt"

func main() {
	ch := make(chan string)

	// YOUR TURN: ek goroutine jo channel me "hello" bheje
	go func() {
		ch <- "Hello"
	}()

	// YOUR TURN: main me receive karo aur print karo
	msg := <-ch
	fmt.Println(msg) // hello
}
