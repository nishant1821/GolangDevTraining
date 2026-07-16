package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	counter := 0

	// 10 goroutines, har ek 1000 baar increment
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				// YOUR TURN: Lock, counter++, Unlock (defer waala yahan mat, loop me hai)
				mu.Lock()
				counter++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	fmt.Println("Final counter:", counter) // 10000 aana chahiye ab
}
