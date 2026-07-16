package main

import (
	"fmt"
	"sync"
)

func square(n int) int {
	return n * n
}

func main() {
	nums := []int{1, 2, 3, 4, 5}
	results := make([]int, len(nums)) // har goroutine apne index pe likhega
	var wg sync.WaitGroup

	for i, n := range nums {
		wg.Add(1)
		go func(idx, val int) {
			defer wg.Done()
			// YOUR TURN: results[idx] me square(val) daalo
			results[idx] = square(val)
			// NOTE: yahan Mutex ki zaroorat NAHI — kyun? (har goroutine ALAG index pe likh raha)

		}(i, n)
	}

	wg.Wait()
	fmt.Println(results) // [1 4 9 16 25]
}
