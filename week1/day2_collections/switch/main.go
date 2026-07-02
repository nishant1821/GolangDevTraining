package main

import (
	"fmt"
	"time"
)

func main() {
	// Simple switch — no fallthrough by default
	i := 5
	switch i {
	case 1:
		fmt.Println("one")
	case 2:
		fmt.Println("two")
	case 5:
		fmt.Println("five")
	default:
		fmt.Println("other")
	}

	// Multiple values per case
	switch time.Now().Weekday() {
	case time.Saturday, time.Sunday:
		fmt.Println("Weekend!")
	default:
		fmt.Println("Weekday")
	}

	// Expressionless switch (like if-else chain)
	score := 75
	switch {
	case score >= 90:
		fmt.Println("A")
	case score >= 75:
		fmt.Println("B")
	case score >= 60:
		fmt.Println("C")
	default:
		fmt.Println("F")
	}

	// Type switch
	whoAmI := func(v interface{}) {
		switch v.(type) {
		case int:
			fmt.Println("int")
		case string:
			fmt.Println("string")
		case bool:
			fmt.Println("bool")
		default:
			fmt.Println("other")
		}
	}
	whoAmI(23)
	whoAmI("hello")
	whoAmI(true)
	whoAmI(3.14)
}
