package main

import "fmt"

type Notifier interface {
	Send(msg string) error
}

type EmailNotifier struct{ from string }
type SMSNotifier struct{ number string }

// YOUR TURN: dono pe Send method likho
func (e EmailNotifier) Send(msg string) error {
	fmt.Println("This is emailNoitifier", msg)
	return nil
}

func (s SMSNotifier) Send(msg string) error {

	fmt.Println("This is sms notifier", msg)
	return nil
}

// YOUR TURN: function jo Notifier leta hai aur Send call karta hai
func SendAlert(n Notifier, msg string) {
	n.Send(msg)

}

func main() {
	// YOUR TURN: dono ke saath SendAlert call karo
	SendAlert(EmailNotifier{}, "Test message")
	SendAlert(SMSNotifier{}, "Test message")
}
