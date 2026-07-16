package main

import (
	"errors"
	"fmt"
)

func repo() error {
	return errors.New("connection timeout")
}

func service() error {
	err := repo()
	if err != nil {
		// YOUR TURN: %w se wrap karo — context: "failed to fetch user"
		return fmt.Errorf("Failed to fetch %w", err)
	}
	return nil
}

func handler() error {
	err := service()
	if err != nil {
		// YOUR TURN: %w se wrap karo — context: "get user request failed"
		return fmt.Errorf("Get user request fail %w", err)
	}
	return nil
}

func main() {
	err := handler()
	fmt.Println(err)
	// Expected: get user request failed: failed to fetch user: connection timeout
}
