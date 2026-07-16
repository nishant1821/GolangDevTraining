package main

import "fmt"

type User struct {
	ID    int
	Name  string
	Email string
	Age   int
}

// YOUR TURN: Greet() — "Hi, I'm <Name>" return kare
func (u User) Greet() string {

	return "Hi I'm " + u.Name
}

// YOUR TURN: IsAdult() bool — Age >= 18 ho to true
func (u User) IsAdult() bool {

	return u.Age >= 10
}

func main() {
	u := User{Name: "Nishant", Age: 25}
	fmt.Println(u.Greet())   // Hi, I'm Nishant
	fmt.Println(u.IsAdult()) // true
}
