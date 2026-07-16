package main

import "fmt"

type Notifier interface{ Send(msg string) error }
type EmailNotifier struct{ from string }

func (e EmailNotifier) Send(msg string) error { return nil }

func main() {
	var n Notifier = EmailNotifier{from: "hello@x.com"}

	// YOUR TURN: safe assertion se EmailNotifier nikaalo (, ok pattern)
	e, ok := n.(EmailNotifier)
	if ok {
		fmt.Println("from:", e.from)
	} else {
		fmt.Println("EmailNotifier nahi hai")
	}
}
