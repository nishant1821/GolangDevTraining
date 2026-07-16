package main

import (
	"fmt"
	"time"
)

func printMsg(msg string) {
	fmt.Println(msg)
}

func main() {
	// YOUR TURN: go se printMsg("from goroutine") call karo
	go printMsg("Hi i am Go routine")

	printMsg("from main")

	// Ye line hataake bhi chala ke dekh — "from goroutine" gayab ho jaayega!
	// (ye ganda hack hai, sirf demo ke liye — asli solution WaitGroup hai)
	time.Sleep(100 * time.Millisecond)
}
