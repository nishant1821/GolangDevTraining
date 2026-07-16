package main

import (
	"errors"
	"fmt"
)

// Sentinel error
var ErrOutOfStock = errors.New("item out of stock")

func checkStock(qty int) error {
	if qty == 0 {
		// YOUR TURN: ErrOutOfStock ko wrap karke return karo (context: "product unavailable")
		return 
	}
	return nil
}

func main() {
	err := checkStock(0)

	// YOUR TURN: errors.Is se check karo ki chain me ErrOutOfStock hai
	if  {
		fmt.Println("Stock khatam — customer ko batao")
	}

	// Bonus: seedha == se compare karke dekh — kya hoga? (wrapped hai isliye false)
	fmt.Println("Direct ==:", err == ErrOutOfStock)   // false! (wrap ki wajah se)
}