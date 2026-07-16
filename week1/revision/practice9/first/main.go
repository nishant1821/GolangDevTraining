package main

import (
	"context"
	"fmt"
	"time"
)

func worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// YOUR TURN: print "worker: cancelled" aur return
			fmt.Println("Worker cancelled ")

		default:
			fmt.Println("worker: kaam kar raha hoon...")
			time.Sleep(300 * time.Millisecond)
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	go worker(ctx)

	time.Sleep(1 * time.Second) // 1 second kaam chalne do
	// YOUR TURN: cancel() call karo — worker rukega
	cancel()

	time.Sleep(100 * time.Millisecond) // worker ko rukने ka time
	fmt.Println("main: done")
}
