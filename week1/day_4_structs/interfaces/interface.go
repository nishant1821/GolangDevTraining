package main

import "fmt"

// Go lang apne aap samjh jaata hai implements nahi krnaa naam same hona chahiye struct automatically smjh jaataa hai ki interface implementkr rhaa hak

type paymenter interface {
	pay(amount float32)
}
type payment struct {
	gateway paymenter
}

func (p payment) makePayment(amount float32) {
	// razorpayPaymentGw := razorpay{}

	p.gateway.pay(amount)
}

type razorpay struct{}

func (r razorpay) pay(amount float32) {
	// logic to make payment
	fmt.Println("making payment using razorpay ", amount)
}

type stripe struct{}

func (s stripe) pay(amount float32) {
	fmt.Println("Making payment using stripe")
}

type fakePayment struct{}

func (f fakePayment) pay(amount float32) {
	fmt.Println(("Making payment using fake payment gateway for testing purpose"))
}

func main() {

	// newPayment := payment{}
	fakeGw := fakePayment{}
	newPayment := payment{
		gateway: fakeGw,
	}

	newPayment.makePayment(100)
}
