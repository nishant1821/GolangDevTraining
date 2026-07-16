package main

import (
	"errors"
	"fmt"
)

// YOUR TURN: NotFoundError struct (ID int) + Error() method
type NotFoundError struct {
	ID int
	error

}
func (e NotFoundError) Error() string {
return  "Not found error " + e.ID
}

// repo layer — asli error yahan janm leta hai
func repoFindUser(id int) error {
	if id == 42 {
		return NotFoundError{ID: id}   // user 42 nahi mila
	}
	return nil
}

// service layer — repo ko call karke wrap karta hai
func serviceGetUser(id int) error {
	err := repoFindUser(id)
	if err != nil {
		// YOUR TURN: %w se wrap karo (context: "service: could not get user")
		return 
	}
	return nil
}

// handler layer — sabse upar, wrap + decide
func handlerGetUser(id int) {
	err := serviceGetUser(id)
	if err == nil {
		fmt.Println("User mil gaya!")
		return
	}

	// YOUR TURN: errors.As se NotFoundError nikaalo
	var nf NotFoundError
	if errors.As( , ) {
		fmt.Printf("404: user %d nahi mila\n", nf.ID)
	} else {
		fmt.Println("500: kuch aur gadbad:", err)
	}
}

func main() {
	handlerGetUser(42)   // 404 aana chahiye, ID 42 ke saath
	handlerGetUser(1)    // User mil gaya
}