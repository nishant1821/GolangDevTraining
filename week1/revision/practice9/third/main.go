package main

import (
	"context"
	"fmt"
	"time"
)

// repo — sabse neeche, DB query simulate karta hai
func repo(ctx context.Context, id int) (string, error) {
	select {
	case <-time.After(2 * time.Second): // query 2s leti hai
		return fmt.Sprintf("user-%d", id), nil
	case <-ctx.Done():
		// YOUR TURN: ctx cancel hua — "", ctx.Err() return karo

		return "", ctx.Err()
	}
}

// service — beech me, ctx aage pass karta hai
func service(ctx context.Context, id int) (string, error) {
	// YOUR TURN: repo ko ctx ke saath call karo, result aur err return karo
	return repo(ctx, id)
}

// handler — sabse upar, ctx banata hai
func handler(id int) {
	// YOUR TURN: 1 second timeout ctx banao (defer cancel!)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result, err := service(ctx, id)
	if err != nil {
		fmt.Println("Error:", err) // timeout (1s) < query (2s) → error
		return
	}
	fmt.Println("Result:", result)
}

func main() {
	handler(42)
	// query 2s leti hai, timeout 1s — to "context deadline exceeded" aayega
}
