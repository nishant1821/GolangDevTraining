package main

import "fmt"

// Array — fixed size, elements of same type.
// Go never assigns garbage values — zero value for int is 0.
func main() {
	var nums [4]int
	nums[0] = 1
	fmt.Println(nums)     // [1 0 0 0]
	fmt.Println(len(nums)) // 4

	// Declare with values
	nums2 := [3]int{1, 2, 3}
	fmt.Println(nums2)

	// Bool array — all false by default
	var flags [4]bool
	fmt.Println(flags) // [false false false false]

	// 2D array
	matrix := [2][2]int{{3, 4}, {5, 6}}
	fmt.Println(matrix)

	// Arrays are value types — copies on assignment
	// Most of the time you'll use slices, not arrays
}
