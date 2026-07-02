package main

import "fmt"

// Exercise: implement filter and mapSlice as higher-order functions.

func filter(nums []int, fn func(int) bool) []int {
	result := []int{}
	for _, n := range nums {
		if fn(n) {
			result = append(result, n)
		}
	}
	return result
}

func mapSlice(nums []int, fn func(int) int) []int {
	result := make([]int, len(nums))
	for i, n := range nums {
		result[i] = fn(n)
	}
	return result
}

func main() {
	nums := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	evens := filter(nums, func(n int) bool { return n%2 == 0 })
	fmt.Println("Evens:", evens)

	odds := filter(nums, func(n int) bool { return n%2 != 0 })
	fmt.Println("Odds:", odds)

	doubled := mapSlice(nums, func(n int) int { return n * 2 })
	fmt.Println("Doubled:", doubled)

	squares := mapSlice(nums, func(n int) int { return n * n })
	fmt.Println("Squares:", squares)

	// Chain: filter evens, then double them
	evenDoubled := mapSlice(filter(nums, func(n int) bool { return n%2 == 0 }), func(n int) int { return n * 2 })
	fmt.Println("Even numbers doubled:", evenDoubled)
}
