package main

import (
	"fmt"
)

// Multiple return values In go we have provided muliple return values
func divide(a, b float64) (float64, error) {
	if b == 0 {
		return 0, fmt.Errorf("cannot divided by zero", a)
	}
	return a / b, nil
}

// Named return values
func split(sum int) (x, y int) {
	x = sum * 4 / 9
	y = sum - x
	return
}

// variadic functions

func sum(nums ...int) int {
	total := 0
	for _, n := range nums {
		total += n
	}
	return total
}

func makeCounter() func() int {
	count := 0

	return func() int {
		count++
		return count
	}
}
func main() {

	// result, err := divide(10, 0)

	// if err != nil {
	// 	log.Fatal(err)
	// }

	// fmt.Println(result)

	fmt.Println(split(432))

	fmt.Println(sum(2, 3, 4, 21, 4, 3))

	c := makeCounter()

	fmt.Println(c(), c(), c())
}
