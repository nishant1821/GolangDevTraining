package main

import (
	"errors"
	"fmt"
)

// Exercise: divide returns (result, error) — error on division by zero.
func divide(a, b float64) (float64, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}

func main() {
	pairs := [][2]float64{
		{10, 2},
		{5, 0},
		{9, 3},
		{7, 0},
	}

	for _, p := range pairs {
		result, err := divide(p[0], p[1])
		if err != nil {
			fmt.Printf("%.0f / %.0f → error: %s\n", p[0], p[1], err)
		} else {
			fmt.Printf("%.0f / %.0f = %.2f\n", p[0], p[1], result)
		}
	}
}
