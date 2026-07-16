package main

import "fmt"

func main() {
	// YOUR TURN: 5 students ka map banao naam→marks
	grades := map[string]int{
		"Amit":    30,
		"Nishant": 98,
		"Piyush":  96,
		"Isha":    99,
		"Naman":   97,
	}

	fmt.Println(grades)
	// YOUR TURN: har student pe ghumo, 40+ = Pass, warna Fail
	// switch (expression-less) ya if-else, dono chalega
	for _, val := range grades {
		switch {
		case val > 40:
			fmt.Println("Pass")
		default:
			fmt.Println("Fail")
		}

	}
}
