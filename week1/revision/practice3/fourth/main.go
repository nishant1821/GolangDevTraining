package main

import "fmt"

func makeCounter() func() int {
	count := 0
	return func() int {
		count++
		return count
	}
}

func main() {
	a := makeCounter()
	b := makeCounter()

	fmt.Println(a()) // 1
	fmt.Println(a()) // 2
	fmt.Println(b()) // ? 1  <- yahan dhyaan de
	fmt.Println(a()) // ? 3 <- aur yahan
}
