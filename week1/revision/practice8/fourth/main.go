package main

import "fmt"

// Stage 1: numbers generate karo, out channel me daalo
func generator(out chan<- int) {
	for i := 1; i <= 5; i++ {
		out <- i
	}
	close(out) // bhejna khatam
}

// Stage 2: in se lo, square karke out me daalo
func squarer(in <-chan int, out chan<- int) {
	// YOUR TURN: range in pe ghumo, out <- n*n bhejo, phir close(out)
	for n := range in {
		out <- n * n
	}
	close(out)
}

// Stage 3: in se lo, print karo
func printer(in <-chan int, done chan<- bool) {
	// YOUR TURN: range in pe ghumo, print karo
	for sq := range in {
		fmt.Println(sq)

	}
	done <- true // khatam ka signal
}

func main() {
	nums := make(chan int)
	squares := make(chan int)
	done := make(chan bool)

	go generator(nums)
	go squarer(nums, squares)
	go printer(squares, done)

	<-done // printer ke khatam hone tak ruko
	fmt.Println("Pipeline done!")
}

// Output: 1 4 9 16 25
