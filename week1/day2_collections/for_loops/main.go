package main

import "fmt"

// for — the only loop construct in Go (replaces while, do-while, for)
func main() {
	// Classic for (like C)
	for i := 0; i <= 4; i++ {
		if i == 3 {
			continue // skip 3
		}
		fmt.Println(i)
	}

	// While-style
	i := 1
	for i <= 3 {
		fmt.Println(i)
		i++
	}

	// Range over number (Go 1.22+)
	for j := range 3 {
		fmt.Println("range:", j)
	}

	// break example
	for k := 0; k < 10; k++ {
		if k == 5 {
			break
		}
		fmt.Println("k:", k)
	}

	// Infinite loop (commented — would run forever)
	// for {
	// 	fmt.Println("forever")
	// }
}
