package main

import "fmt"

// Exercise: demonstrate defer — execution order and file-open/close simulation.
//
// defer runs when the surrounding function returns, in LIFO order.

func openFile(name string) {
	fmt.Println("Opening:", name)
	defer fmt.Println("Closing:", name) // scheduled now, runs on return

	fmt.Println("Reading from:", name)
	fmt.Println("Processing data in:", name)
	// No matter how the function exits, the file always gets "closed"
}

func lifoOrder() {
	fmt.Println("\n--- defer runs in LIFO (last-in, first-out) order ---")
	defer fmt.Println("first defer  → runs last")
	defer fmt.Println("second defer → runs second")
	defer fmt.Println("third defer  → runs first")
	fmt.Println("function body")
}

func deferLoop() {
	fmt.Println("\n--- defer inside a loop ---")
	for i := 0; i < 3; i++ {
		defer fmt.Println("deferred loop i =", i) // each iteration registers separately
	}
	fmt.Println("loop done")
}

func main() {
	openFile("data.txt")
	lifoOrder()
	deferLoop()
}
