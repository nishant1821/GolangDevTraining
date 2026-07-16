package main

import "fmt"

func main() {
	// YOUR TURN: size 2 ka buffered channel banao
	ch := make(chan string, 3)

	ch <- "first"  // nahi rukega
	ch <- "second" // nahi rukega
	ch <- "third"  // ye rukega — SOCH kyun?

	fmt.Println(<-ch) // first
	fmt.Println(<-ch) // second
}
