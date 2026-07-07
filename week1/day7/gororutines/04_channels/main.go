package main

import "fmt"

func sum(nums []int, ch chan int) {
	total := 0
	for _, n := range nums {
		total += n
	}
	ch <- total // send result into channel
}

func main() {
	nums := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	ch := make(chan int)

	// Split work across two goroutines
	go sum(nums[:5], ch)
	go sum(nums[5:], ch)

	a, b := <-ch, <-ch // receive both results (order may vary)
	fmt.Println("partial sums:", a, b)
	fmt.Println("total:", a+b)

	// Buffered channel — send does not block until buffer is full
	buffered := make(chan string, 2)
	buffered <- "hello"
	buffered <- "world"
	fmt.Println(<-buffered)
	fmt.Println(<-buffered)
}
