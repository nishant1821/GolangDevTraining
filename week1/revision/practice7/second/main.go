package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup

	for i := 1; i <= 5; i++ {
		// YOUR TURN: Add(1) karo
		wg.Add(1)

		go func(id int) {
			// YOUR TURN: defer se Done
			defer wg.Done()

			fmt.Println("Goroutine", id, "chal raha hai")
		}(i)
	}

	// YOUR TURN: sab ke khatam hone tak ruko
	wg.Wait()
	fmt.Println("Sab done!")
	// NOTE: order har baar alag aayega — ye NORMAL hai (concurrent hai)
}
