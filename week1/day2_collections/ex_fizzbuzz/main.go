package main

import "fmt"

// Exercise: FizzBuzz using switch (not if/else).
// Rules: multiples of 3 → Fizz, multiples of 5 → Buzz, both → FizzBuzz.
func fizzBuzz(n int) string {
	switch {
	case n%15 == 0:
		return "FizzBuzz"
	case n%3 == 0:
		return "Fizz"
	case n%5 == 0:
		return "Buzz"
	default:
		return fmt.Sprintf("%d", n)
	}
}

func main() {
	for i := 1; i <= 30; i++ {
		fmt.Println(fizzBuzz(i))
	}
}
