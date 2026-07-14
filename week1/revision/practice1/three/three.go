package main

import "fmt"

func main() {
	const speedOfLight = 299792458 // meters/second

	var distance float64 = 384400000 // Earth to Moon (meters)

	// YOUR TURN: time nikalo = distance / speed
	// Socho: dono ko float me convert karna padega?
	timeInSeconds := distance / float64(speedOfLight)

	fmt.Printf("Light takes %.2f seconds to reach the Moon\n", timeInSeconds)
}
