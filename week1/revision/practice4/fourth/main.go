package main

import "fmt"

type User struct {
	ID   int
	Name string
	Age  int
}

func (u User) Greet() string {
	return "Hi, I'm " + u.Name
}

func (a Admin) Greet() string {
	return "Hi, I'm admin greet " + a.Name
}

// YOUR TURN: Admin struct — User embed karo + Role(string) field add karo
type Admin struct {
	User
	Role string
}

func main() {
	a := Admin{
		User: User{ID: 1, Name: "Nishant", Age: 25},
		Role: "superadmin",
	}

	// YOUR TURN: seedha a.Name print karo (User ka field, promoted)
	fmt.Println(a.Name)

	// YOUR TURN: a.Greet() call karo (User ka method, promoted)
	fmt.Println(a.Greet())

	// YOUR TURN: a.Role print karo
	fmt.Println(a.Role)
}
