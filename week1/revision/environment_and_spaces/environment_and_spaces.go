package main // every runnable program lives in package main

import (
	"fmt" // fmt = formatting/printing, from the standard library
	"os"
	"strconv"
	// "time"
)

func main() { // main() is the entry point — execution starts here
	// fmt.Println("Hello, Go!")

	// name := "Nishant"
	// fmt.Println("Hello ", name, time.Now())
	// // YOUR TURN: print "Hello, <name>! Today is <date>."
	// // Hint: time.Now() gives the current time; .Format("2006-01-02") formats a date.

	if len(os.Args) < 2 {
		fmt.Println("usage: go run main.go <celsius>")
		return
	}
	c, err := strconv.ParseFloat(os.Args[1], 64) // parse the CLI arg to float64
	if err != nil {
		fmt.Println("please pass a number")
		return
	}
	// Celsius → Fahrenheit:  F = C*9/5 + 32
	// Celsius → Kelvin:      K = C + 273.15

	f := c*9/5 + 32
	k := c + 273.15
	fmt.Printf("%.2f°C = %.2f°F = %.2fK\n", c, f, k)

}
