package main

import "fmt"

func main() {
	// Way 1: explicit type
	var age int = 30
	var name string = "Alice"

	// Way 2: type inference
	var age2 = 35
	var name2 = "Nishant"

	// Way 3: short declaration (only inside functions)
	age3 := 33
	name3 := "Nishant 3"

	a, b, c := "Olive", 30, true
	fmt.Println(age, name, age2, name2, age3, name3, a, b, c)

	// Zero values — Go's safety net
	var count int     // 0
	var price float64 // 0.0
	var label string  // ""
	var ready bool    // false
	fmt.Println(count, price, label, ready)

	// Constants
	const Pi = 3.14144
	const appName = "GolangTraining"
	fmt.Println(Pi, appName)

	const (
		StatusActive   = "active"
		StatusInactive = "inactive"
		MaxRetries     = 3
	)
	fmt.Println(StatusActive, StatusInactive, MaxRetries)
}
