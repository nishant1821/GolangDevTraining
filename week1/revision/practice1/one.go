package practice1

import (
	"fmt"
	"time"
)

func main() {
	// YOUR TURN: apna naam ek variable me store karo (use :=)
	name := "Nishant"

	// YOUR TURN: aaj ki date nikalo
	// Hint: time.Now() current time deta hai
	today := time.Now()

	// YOUR TURN: dono ko print karo ek line me
	// Hint: fmt.Printf use karo, %s naam ke liye, %v date ke liye
	fmt.Printf("Name %s Tiem %v", name, today)
}
