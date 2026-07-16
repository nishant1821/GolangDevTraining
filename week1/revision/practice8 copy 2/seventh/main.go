package main

import (
	"fmt"
	"slices"
)

func main() {
	original := []string{"a", "b", "c"}

	// YOUR TURN: slices.Clone se copy banao
	dup := slices.Clone(original)

	// YOUR TURN: dup ke pehle element ko "Z" kar do
	dup[0] = "d"
	// dono print karo — original safe hona chahiye
	fmt.Println("original:", original) // [a b c] hona chahiye
	fmt.Println("dup:", dup)           // [Z b c]
}
