package main

import "fmt"

func main() {
	for i := 1; i <= 20; i++ {
		// YOUR TURN: switch (expression-less) use karo
		// Rules:
		//   3 aur 5 dono se divisible → "FizzBuzz"
		//   sirf 3 se → "Fizz"
		//   sirf 5 se → "Buzz"
		//   warna → number khud
		// Hint: i%3 == 0 matlab 3 se divisible
		// Hint: FizzBuzz case SABSE UPAR aana chahiye — kyun? socho.
		switch {
		case i%3 == 0 && i%5 == 0:
			fmt.Println("FizzBuzz", i)
		case i%3 == 0:
			fmt.Println("Fizz", i)
		case i%5 == 0:
			fmt.Println("Buzz", i)
		default:
			fmt.Println()
		}
	}
}
