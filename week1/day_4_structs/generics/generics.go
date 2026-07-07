package main

import "fmt"

func printSlice[T comparable, V string](items []T, name V) {
	fmt.Println(name)
	for _, item := range items {
		fmt.Println(item)
	}
}

//	func printSliceString(items []string) {
//		for _, item := range items {
//			fmt.Println(item)
//		}
//	}
//
// LIFO
type Stack[T any] struct {
	elements []T
}

func main() {
	// names := []string{"golang", "typescript"}
	// nums := []int{1, 2, 4, 5}
	// vals := []bool{true, false, true}
	// printSlice(names)
	// printSlice(nums)
	// printSlice(vals)

	myStack := Stack[int]{
		elements: []int{2, 3, 4, 5},
	}
	myStack2 := Stack[string]{
		elements: []string{"abc", "bcd", "abse"},
	}
	fmt.Println(myStack)
	fmt.Println(myStack2)
}
