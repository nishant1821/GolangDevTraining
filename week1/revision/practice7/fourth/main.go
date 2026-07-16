package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup

	for i := 1; i <= 3; i++ {
		wg.Add(1)

		go func(n int) {
			defer wg.Done()
			fmt.Println("goroutine", n)
		}(i)
	}
	// YOUR TURN: yahan kuch nahi likho, chala ke dekho
	wg.Wait()
	// SOCH: kya print hoga? (shayad kuch bhi nahi — main turant khatam!)
	fmt.Println("main done")
}

// Phir WaitGroup add karke fix karo
