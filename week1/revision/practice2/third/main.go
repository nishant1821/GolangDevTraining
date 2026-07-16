package main

import "fmt"

func main() {
	prices := []float64{10.5, 20.0, 5.25, 100.0}

	// YOUR TURN: range use karke total sum nikalo
	total := 0.0
	for _, val := range prices {
		total += val

	}
	fmt.Printf("Total: %.2f\n", total)

	// YOUR TURN: sirf INDEX chahiye (value nahi) — value ko ignore karo
	// har index print karo
	for idx, _ := range prices {
		fmt.Println("Index:", idx)
	}
}
