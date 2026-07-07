package main

import (
	"fmt"
	"strings"
)

func main() {
	sentence := "the cat sat on the mat the cat ran"
	words := strings.Fields(sentence) // splits on whitespace → []string

	counts := make(map[string]int)
	// YOUR TURN: range over words; increment counts[word] for each
	// (a missing key reads as 0, so counts[word]++ just works)
	for _, word := range words {
		counts[word]++
	}
	fmt.Println(counts) // expect: the:3 cat:2 sat:1 on:1 mat:1 ran:1
	_ = words

	for i := 1; i <= 20; i++ {
		// YOUR TURN: use a switch { } (no expression) with cases:
		//   i%15==0 → "FizzBuzz", i%3==0 → "Fizz", i%5==0 → "Buzz", default → the number
		_ = i
		switch {
		case i%15 == 0:
			fmt.Println("FuzzBuzz")
		case i%3 == 0:
			fmt.Println("Fizz")
		case i%5 == 0:
			fmt.Println("Fizz")
		default:
			fmt.Println("the number")

		}

	}

	grades := map[string]int{
		"Asha": 82, "Ben": 47, "Cara": 91, "Dev": 55, "Eli": 38,
	}
	// YOUR TURN: range the map; print "<name>: PASS" if >= 50 else "<name>: FAIL"
	_ = grades

	for _, val := range grades {

		if val >= 50 {
			fmt.Println("pass")
		} else {
			fmt.Println("Fail")
		}

	}
}
