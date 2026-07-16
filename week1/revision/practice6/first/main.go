package main

import (
	"errors"
	"fmt"
)

// YOUR TURN: checkAge — agar age < 0 to error do "age cannot be negative"
// warna nil
func checkAge(age int) error {
	if age < 0 {
		return errors.New("Age cannot be below then 18")
	}
	return nil
}

func main() {
	// YOUR TURN: checkAge(-5) call karo, err check karo
	err := checkAge(-5)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Println("Valid age")
	}

	// YOUR TURN: checkAge(25) — ab error nahi aana chahiye

}
