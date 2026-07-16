package main

import (
	"context"
	"fmt"
	"time"
)

// Slow kaam — 3 second lagta hai
func slowOperation(ctx context.Context, done chan<- bool) {
	select {
	case <-time.After(3 * time.Second): // kaam 3s me hota
		fmt.Println("kaam pura hua")
		done <- true
	case <-ctx.Done(): // ctx cancel/timeout hua
		// YOUR TURN: print "kaam cancel hua:", ctx.Err()
		fmt.Println("Cancel")
		ctx.Err()
		done <- false
	}
}

func main() {
	// YOUR TURN: 2 second ka timeout context banao (defer cancel bhi!)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan bool)
	go slowOperation(ctx, done)

	<-done
	// kaam 3s, timeout 2s — to "cancel hua" chalega
}
