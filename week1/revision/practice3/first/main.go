package main

import (
	"errors"
	"fmt"
)

// YOUR TURN: divide banao jo (float64, error) return kare
// b == 0 pe error do, warna result aur nil
func divide(a, b float64) (float64, error) {
	if b == 0 {
		return 0, errors.New("It is not dividisble by 0")
	}
	return a / b, nil

}

func main() {
	// Case 1: normal
	result, err := divide(10, 2)
	// YOUR TURN: err check karo, phir result print karo

	if err != nil {
		// err != nil -> matlab divide() ne error diya (Python: `except`, Node: `if (err)` in callback style)
		fmt.Println(err)
		return // bare return -> main() koi value return nahi karta, isliye sirf "return" likha, "return err" nahi
	}
	fmt.Println(result)

	// Case 2: divide by zero
	result, err = divide(10, 0)
	// YOUR TURN: yahan err aana chahiye — usse print karo
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(result)

}
