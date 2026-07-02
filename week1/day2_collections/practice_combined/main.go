package main

import "fmt"

// Combined practice from Day 2 — arrays, slices, maps, loops together
func main() {
	// Arrays
	var arr [3]int
	arr[0], arr[1], arr[2] = 10, 20, 3
	fmt.Println(arr)

	colors := [3]string{"red", "green"}
	fmt.Println(colors)

	// Slices
	fruits := []string{"apple", "banana"}
	fruits = append(fruits, "cherry")
	fmt.Println(fruits, len(fruits))

	s := make([]int, 2, 5)
	s = append(s, 99)
	fmt.Println(s, "len:", len(s), "cap:", cap(s))

	nums := []int{10, 20, 30, 40, 50}
	fmt.Println(nums[1:3], nums[:2], nums[3:])

	// Maps
	grades := map[string]int{"Alice": 90, "Bob": 75}
	grades["Alice"] = 95
	grades["Charlie"] = 60
	delete(grades, "Bob")
	for name, score := range grades {
		fmt.Printf("%s: %d\n", name, score)
	}

	// For loops
	for i := 0; i < 5; i++ {
		fmt.Print(i, " ")
	}
	fmt.Println()

	n := 1
	for n < 100 {
		n *= 2
	}
	fmt.Println("first power of 2 >= 100:", n)
}
