package main

import "fmt"

func main() {
	inventory := map[string]int{
		"apples":  50,
		"bananas": 30,
	}

	// YOUR TURN: "mangoes" ko 20 quantity ke saath add karo
	inventory["mangoes"] = 20

	fmt.Println(inventory)
	// YOUR TURN: check karo "oranges" hai ya nahi, comma-ok se
	// agar nahi to print "Out of stock"
	qty, ok := inventory["oranges"]
	if ok {
		fmt.Println("oranges:", qty)
	} else {
		fmt.Println("Out of stock")
	}

	// YOUR TURN: "bananas" delete karo
	delete(inventory, "oranges")

}
