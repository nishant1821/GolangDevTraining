package main

import "fmt"

func main() {
	fmt.Println("start")
	// YOUR TURN: teen defer likho jo print karein "cleanup 1", "cleanup 2", "cleanup 3"
	defer fmt.Println("Cleanup 1")
	defer fmt.Println("Cleanup 2")
	defer fmt.Println("Cleanup 3")

	// SOCHO: output me ye kis order me aayenge?

	fmt.Println("end")
}

// Pehle SOCH ke likh ki output kya hoga, phir run karke check kar
