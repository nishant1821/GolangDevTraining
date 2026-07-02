package main

import (
	"fmt"
	"maps"
)

// Map — hash map (like dict in Python, object in JS)
func main() {
	// Create with make
	m := make(map[string]string)
	m["name"] = "golang"
	m["area"] = "backend"
	fmt.Println(m)
	fmt.Println(len(m))

	// Delete a key
	delete(m, "area")
	fmt.Println(m)

	// Map literal
	grades := map[string]int{
		"Alice": 90,
		"Bob":   75,
	}

	// Read
	fmt.Println(grades["Alice"]) // 90

	// Update / Add
	grades["Alice"] = 95   // update
	grades["Charlie"] = 60 // add new

	// Delete
	delete(grades, "Bob")

	// Iterate
	for name, score := range grades {
		fmt.Printf("%s: %d\n", name, score)
	}

	// Check if key exists (comma-ok idiom)
	price := map[string]int{"apple": 40, "banana": 20}
	val, ok := price["apple"]
	if ok {
		fmt.Println("apple price:", val)
	} else {
		fmt.Println("key not found")
	}

	// Compare two maps
	m1 := map[string]int{"a": 1, "b": 2}
	m2 := map[string]int{"a": 1, "b": 2}
	fmt.Println("maps equal:", maps.Equal(m1, m2))
}
