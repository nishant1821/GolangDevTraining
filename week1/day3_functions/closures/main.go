package main

import "fmt"

// Closure — a function that captures variables from its outer scope.
func makeCounter() func() int {
	count := 0
	return func() int {
		count++
		return count
	}
}

// Each call to makeCounter() creates an independent counter.
func makeAdder(x int) func(int) int {
	return func(y int) int {
		return x + y
	}
}

func main() {
	counter := makeCounter()
	fmt.Println(counter()) // 1
	fmt.Println(counter()) // 2
	fmt.Println(counter()) // 3

	// Independent instance
	counter2 := makeCounter()
	fmt.Println(counter2()) // 1 — its own count

	add5 := makeAdder(5)
	fmt.Println(add5(3))  // 8
	fmt.Println(add5(10)) // 15
}
