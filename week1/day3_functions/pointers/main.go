package main

import "fmt"

// Pass by value — caller's variable is unchanged
func changeNum(num int) {
	num = 5
	fmt.Println("inside changeNum:", num)
}

// Pass by pointer — modifies the caller's variable
func changeNumByPointer(num *int) {
	*num = 5 // dereference to set the value
	fmt.Println("inside changeNumByPointer:", *num)
}

func main() {
	num := 1
	fmt.Println("memory address of num:", &num)

	changeNum(num)
	fmt.Println("after changeNum:", num) // still 1

	changeNumByPointer(&num)
	fmt.Println("after changeNumByPointer:", num) // now 5

	// Pointer basics
	x := 42
	p := &x   // p holds the address of x
	fmt.Println(*p) // dereference: prints 42
	*p = 100        // change x through the pointer
	fmt.Println(x)  // 100
}
