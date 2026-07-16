package main

import "fmt"

func main() {
	scores := []int{85, 90, 78}

	// YOUR TURN: 95 aur 88 append karo (ek hi append call me dono daal sakte ho)
	scores = append(scores, 95, 98)

	// YOUR TURN: length aur capacity dono print karo
	fmt.Printf("len=%d cap=%d\n", len(scores), cap(scores))

	// // YOUR TURN: pehle 2 elements ka ek slice banao aur print karo
	firstTwo := scores[0:2]
	fmt.Println(firstTwo)
}
