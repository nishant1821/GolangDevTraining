package main

import "fmt"

// Exercise: store 5 students with scores in a map, print pass/fail.
// Pass mark: 50
func main() {
	grades := map[string]int{
		"Alice":   85,
		"Bob":     45,
		"Charlie": 72,
		"Diana":   30,
		"Eve":     90,
	}

	fmt.Println("Student Results:")
	fmt.Println("----------------------------")
	for name, grade := range grades {
		result := "FAIL"
		if grade >= 50 {
			result = "PASS"
		}
		fmt.Printf("  %-10s %3d  →  %s\n", name, grade, result)
	}
}
