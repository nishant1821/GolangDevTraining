package main

import "fmt"

// range — iterates over arrays, slices, maps, strings, channels
func main() {
	nums := []int{6, 7, 8}

	// with index
	sum := 0
	for i, num := range nums {
		sum += num
		fmt.Println(i, num)
	}
	fmt.Println("sum:", sum)

	// skip index with _
	for _, num := range nums {
		fmt.Println(num)
	}

	// range over map
	m := map[string]string{"fname": "Nishant", "lname": "Gangwar"}
	for k, v := range m {
		fmt.Println(k, "=", v)
	}

	// range over string (gives runes)
	for i, ch := range "Go!" {
		fmt.Println(i, string(ch))
	}
}
