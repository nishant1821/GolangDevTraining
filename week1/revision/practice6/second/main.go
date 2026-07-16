package main

import "fmt"

// YOUR TURN: NotFoundError struct — ID(int) field
// Error() method: "user with id <ID> not found"
type NotFoundError struct {
	ID int
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("This is the error for Id ", e.ID)

}

// YOUR TURN: ValidationError struct — Field(string), Message(string)
// Error() method: "validation failed on <Field>: <Message>"
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation failed on %s: %s", e.Field, e.Message)
}

func main() {
	var err1 error = NotFoundError{ID: 42}
	var err2 error = ValidationError{Field: "email", Message: "must contain @"}

	fmt.Println(err1) // user with id 42 not found
	fmt.Println(err2) // validation failed on email: must contain @
}
