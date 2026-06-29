package main

import "fmt"

func main() {
	fmt.Println("Hello world")

	// Way 1
	var age int = 30
	var name string = "Alice"

	// Way 2
	var age2 = 35
	var name2 = "Nishant"

	// Way 3
	age3 := 33
	name3 := "Nishant 3"

	a, b, c := "Olive", 30, true
	fmt.Println(age, name, age2, name2, age3, name3, a, b, c)

	// Zero values go safety net
	var count int     // 0
	var price float64 // 0.0
	var label string  // ""  (empty string)
	var ready bool    // false

	fmt.Println(count, price, label, ready)

	const Pi = 3.14144
	const appName = "AppName"

	// Pi = 3242  -> We cannot reassign the values into this

	const (
		StatusActive   = "active"
		StatusInactive = "inactive"
		MaxRetries     = 3
	)
	fmt.Println(StatusActive, StatusInactive, MaxRetries)

}
