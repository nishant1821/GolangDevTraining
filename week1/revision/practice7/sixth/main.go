package main

import (
	"fmt"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[string]int{} // shared map — map concurrent-safe NAHI hai!

	words := []string{"go", "go", "rust", "go", "rust", "python"}

	for _, w := range words {
		wg.Add(1)
		go func(word string) {
			defer wg.Done()
			mu.Lock()
			defer mu.Unlock()
			// YOUR TURN: Lock, counts[word]++, Unlock
			counts[word]++
			// (map pe bina lock likhna Go me PANIC de sakta hai — "concurrent map writes")

		}(w)
	}

	wg.Wait()
	fmt.Println(counts) // map[go:3 python:1 rust:2]
}
