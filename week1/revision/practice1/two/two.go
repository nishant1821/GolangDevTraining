package main

import "fmt"

func main() {
	// Ek celsius value se shuruaat
	celsius := 37.0

	// YOUR TURN: Celsius se Fahrenheit
	// Formula: (C × 9/5) + 32
	// DHYAAN: 9/5 integer division hai = 1! Isko 9.0/5.0 likho
	fahrenheit := (celsius * 9 / 5) + 32

	// YOUR TURN: Celsius se Kelvin
	// Formula: C + 273.15
	kelvin := celsius + 273.15

	// YOUR TURN: teeno print karo saaf-suthre format me
	// Chahiye: "37.0°C = 98.6°F = 310.15K"
	fmt.Printf("Celcius : %g", celsius)
	fmt.Printf("Faheranite : %v", fahrenheit)

	fmt.Printf("kelvin : %v", kelvin)

}
