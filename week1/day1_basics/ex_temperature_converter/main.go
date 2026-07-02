package main

import (
	"fmt"
	"os"
	"strconv"
)

// Exercise: CLI temperature converter — go run main.go <value> <unit>
// Units: C (Celsius), F (Fahrenheit), K (Kelvin)
// Example: go run main.go 100 C

func celsiusToFahrenheit(c float64) float64 { return c*9/5 + 32 }
func fahrenheitToCelsius(f float64) float64 { return (f - 32) * 5 / 9 }
func celsiusToKelvin(c float64) float64     { return c + 273.15 }
func kelvinToCelsius(k float64) float64     { return k - 273.15 }

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: go run main.go <value> <unit>")
		fmt.Println("Units: C, F, K")
		fmt.Println("Example: go run main.go 100 C")
		os.Exit(1)
	}

	val, err := strconv.ParseFloat(os.Args[1], 64)
	if err != nil {
		fmt.Println("Invalid number:", os.Args[1])
		os.Exit(1)
	}

	unit := os.Args[2]

	switch unit {
	case "C":
		fmt.Printf("%.2f°C = %.2f°F\n", val, celsiusToFahrenheit(val))
		fmt.Printf("%.2f°C = %.2fK\n", val, celsiusToKelvin(val))
	case "F":
		c := fahrenheitToCelsius(val)
		fmt.Printf("%.2f°F = %.2f°C\n", val, c)
		fmt.Printf("%.2f°F = %.2fK\n", val, celsiusToKelvin(c))
	case "K":
		c := kelvinToCelsius(val)
		fmt.Printf("%.2fK = %.2f°C\n", val, c)
		fmt.Printf("%.2fK = %.2f°F\n", val, celsiusToFahrenheit(c))
	default:
		fmt.Println("Unknown unit:", unit, "— use C, F, or K")
		os.Exit(1)
	}
}
