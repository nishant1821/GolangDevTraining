package main

import "fmt"

func main() {
	age := 18
	if age >= 18 {
		fmt.Println("Adult")
	} else {
		fmt.Println("Not an adult")
	}

	// else-if chain
	age2 := 16
	if age2 >= 18 {
		fmt.Println("Adult")
	} else if age2 >= 12 {
		fmt.Println("Teenager")
	} else {
		fmt.Println("Kid")
	}

	// Logical operators
	role := "admin"
	hasPermission := true
	if role == "admin" && hasPermission {
		fmt.Println("Access granted")
	}

	// Scoped variable in if — age3 only exists inside this block
	if age3 := 15; age3 >= 18 {
		fmt.Println("Adult", age3)
	} else if age3 >= 12 {
		fmt.Println("Teenager", age3)
	}

	// Go has no ternary operator — use if/else
}
