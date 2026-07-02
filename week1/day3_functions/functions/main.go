package main

import "fmt"

// Basic function
func add(a int, b int) int {
	return a + b
}

// Multiple return values
func getLanguages() (string, string, string) {
	return "golang", "javascript", "c"
}

// Functions as values (first-class functions)
func processIt() func(a int) int {
	return func(a int) int {
		return a * 2
	}
}

// Accept a function as argument
func apply(nums []int, fn func(int) int) []int {
	result := make([]int, len(nums))
	for i, n := range nums {
		result[i] = fn(n)
	}
	return result
}

func main() {
	fmt.Println(add(2, 5))

	lang1, lang2, lang3 := getLanguages()
	fmt.Println(lang1, lang2, lang3)

	double := processIt()
	fmt.Println(double(6)) // 12

	nums := []int{1, 2, 3, 4}
	tripled := apply(nums, func(n int) int { return n * 3 })
	fmt.Println(tripled)
}
