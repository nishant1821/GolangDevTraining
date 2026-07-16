package main

import "fmt"

type Counter struct {
	value int
}

// YOUR TURN: IncValue() — value receiver, value++ karo
func (c Counter) IncValue() {
	c.value++
}

// YOUR TURN: IncPointer() — pointer receiver, value++ karo
func (c *Counter) IncPointer() {
	c.value++

}

func main() {
	c := Counter{value: 0}

	c.IncValue()
	fmt.Println(c.value) // SOCH: kya aayega? 0 ya 1?  0

	c.IncPointer()
	fmt.Println(c.value) // SOCH: ab kya? 1
}

// Pehle guess likh, phir run kar
