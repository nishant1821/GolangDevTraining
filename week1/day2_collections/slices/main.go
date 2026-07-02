package main

import "fmt"

// Slice — dynamic, most-used construct in Go.
func main() {
	// Uninitialized slice is nil
	var nums []int
	fmt.Println(nums == nil) // true
	fmt.Println(len(nums))   // 0

	// make([]T, length, capacity)
	s := make([]int, 2, 5)
	fmt.Println(s)      // [0 0]
	fmt.Println(len(s)) // 2
	fmt.Println(cap(s)) // 5

	s = append(s, 1, 2, 3) // [0 0 1 2 3]
	fmt.Println(cap(s))    // still 5 — no resize needed

	s = append(s, 4) // triggers resize; cap grows
	fmt.Println(cap(s))

	// Slice literals
	fruits := []string{"apple", "banana", "cherry"}
	fmt.Println(fruits)

	// Slicing (sub-slices)
	nums2 := []int{10, 20, 30, 40, 50}
	fmt.Println(nums2[1:3]) // [20 30]
	fmt.Println(nums2[:2])  // [10 20]
	fmt.Println(nums2[3:])  // [40 50]
}
