package main

import (
	"errors"
	"fmt"
	"strings"
)

type User struct {
	Name  string
	Email string
	Age   int
}

// YOUR TURN: NewUser constructor
// Validation:
//   - Name khaali nahi ho
//   - Age 0 se 150 ke beech ho
//   - Email me "@" ho (hint: strings.Contains)
//
// Return (*User, error)
func NewUser(name, email string, age int) (*User, error) {
	if name == "" {
		return nil, errors.New("Name should not be empty.")

	}
	if age < 0 || age > 150 {
		return nil, errors.New("Age should be between 0 to 150")

	}
	if !strings.Contains(email, "@") {
		return nil, errors.New("Email should contain @")
	}

	return &User{
		Name:  name,
		Email: email,
		Age:   age,
	}, nil

}

func main() {
	// Valid case
	u, err := NewUser("Nishant", "n@example.com", 25)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("Created: %+v\n", u)
	}

	// Invalid case — galat email
	_, err = NewUser("Rahul", "no-at-sign", 30)
	fmt.Println("Error:", err) // error aana chahiye
}
