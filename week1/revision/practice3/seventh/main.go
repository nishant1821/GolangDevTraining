package main

import "fmt"

// YOUR TURN: filter banao — sirf wo elements rakho jinpe test() true de
func filter(nums []int, test func(int) bool) []int {
	result := []int{}
	// YOUR TURN: loop, agar test(n) true to append
	for _, val := range nums {
		if test(val) {
			result = append(result, val)
		}
	}

	return result
}

func main() {
	nums := []int{1, 2, 3, 4, 5, 6, 7, 8}

	// Sirf even numbers
	isEven := func(n int) bool { return n%2 == 0 }
	fmt.Println(filter(nums, isEven)) // [2 4 6 8]

	// YOUR TURN: 5 se bade numbers filter karo (inline function likho)

	fmt.Println(filter(nums, func(n int) bool { return n > 5 })) // [6 7 8]
}
