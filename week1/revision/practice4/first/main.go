package main

import "fmt"

// YOUR TURN: User struct define karo — ID(int), Name(string), Email(string), Age(int)
type User struct {
	ID    int
	Name  string
	Email string
	Age   int
}

func main() {
	// YOUR TURN: field names ke saath ek user banao
	u := User{
		ID:    1,
		Name:  "Nishant",
		Email: "Nishant@gmail.com",
		Age:   12,
	}
	fmt.Printf("%+v\n", u) // %+v field names ke saath print karta hai
}
