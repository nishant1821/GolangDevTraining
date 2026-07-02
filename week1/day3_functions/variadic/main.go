package main

import "fmt"

// Variadic function — accepts any number of arguments (...T)
func sum(nums ...int) int {
	total := 0
	for _, n := range nums {
		total += n
	}
	return total
}

func printAll(sep string, items ...string) {
	for i, item := range items {
		if i > 0 {
			fmt.Print(sep)
		}
		fmt.Print(item)
	}
	fmt.Println()
}

func main() {
	fmt.Println(sum(1, 2, 3))         // 6
	fmt.Println(sum(2, 3, 4, 5, 52))  // 66

	// Spread a slice into a variadic function with ...
	nums := []int{2, 4, 5, 6, 7}
	fmt.Println(sum(nums...))

	printAll(", ", "go", "python", "js")
}
