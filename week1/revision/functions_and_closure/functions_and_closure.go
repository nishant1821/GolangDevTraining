package main

import "fmt"

// 1. divide → (result, error); error when b == 0
func divide(a, b float64) (float64, error) {
	if b == 0 {
		return 0, fmt.Errorf("You cann ot print with zero", nil)
	}
	return a / b, nil
}

// 2. makeCounter → a closure that counts up
func makeCounter() func() int {
	// YOUR TURN
	count := 0
	return func() int {
		count++
		return count
	}
	return nil
}

// 3. filter → keep elements where keep(n) is true (first-class function!)
func filter(nums []int, keep func(int) bool) []int {
	// YOUR TURN
	var result []int
	for _, n := range nums {
		if keep(n) {
			result = append(result, n)
		}
	}
	return result
}

// 4. mapInts → apply transform to each element
func mapInts(nums []int, transform func(int) int) []int {
	// YOUR TURN
	var transforms []int
	for _, n := range nums {
		transforms = append(transforms, transform(n))
	}
	return transforms
}

// 5. defer demo: print "open", do work, ensure "close" runs last
func processFile(name string) {
	fmt.Println("open", name)
	// YOUR TURN: defer the "close" line
	defer fmt.Println("closing", name)
	fmt.Println("working on", name)
}

func main() {
	fmt.Println(divide(10, 2)) // 5 <nil>
	fmt.Println(divide(1, 0))  // 0 <error>
	c := makeCounter()
	fmt.Println(c(), c(), c()) // 1 2 3
	nums := []int{1, 2, 3, 4, 5, 6}
	fmt.Println(filter(nums, func(n int) bool { return n%2 == 0 })) // [2 4 6]
	fmt.Println(mapInts(nums, func(n int) int { return n * n }))    // [1 4 9 16 25 36]
	processFile("data.txt")
}
