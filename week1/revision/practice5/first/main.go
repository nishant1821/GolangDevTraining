package main

import "fmt"

// YOUR TURN: Notifier interface define karo — Send(msg string) error method ho
type Notifier interface {
	Send(msg string) error
}

type EmailNotifier struct {
	from string
}

// YOUR TURN: EmailNotifier pe Send method likho
// print karo "Email from <from>: <msg>", return nil
func (e EmailNotifier) Send(msg string) error {

	fmt.Println("This is a message", msg, e.from)
	return nil
}

func main() {
	// YOUR TURN: EmailNotifier banao, uska Send call karo
	e := EmailNotifier{from: "app@x.com"}
	e.Send("Hello!")
}
