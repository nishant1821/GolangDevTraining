package main

import "fmt"

type User struct{ Name string }

// YOUR TURN: type switch jo handle kare int, string, User, aur unknown (default)
func describe(i any) {
	switch v := i.(type) {
	case int:
		fmt.Println("This is int type", v*2)

	case string:
		fmt.Println("This is string type", len(v))

	case User:
		fmt.Println("this is user ", v.Name)

	default:
		fmt.Printf("default", v)

	}
}

func main() {
	describe(42)
	describe("hello")
	describe(User{Name: "Nishant"})
	describe(3.14) // ye default me jaayega
}
