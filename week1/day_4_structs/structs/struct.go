package main

import (
	"fmt"
	"time"
)

type order struct {
	id        string
	amount    float32
	status    string
	createdAt time.Time // nanosecond precision
	customer
}

// reciever type hotaa ye struct mein function ko attached kr deta hai
// we need to pass address to change the type
func (o *order) changeStatus(status string) {
	o.status = status
}

type customer struct {
	name string
	id   int
}

func newOrder(id string, amount float32, status string) *order {
	// initial setup goes here.....
	myOrder := order{
		id:     id,
		amount: amount,
		status: status,
		customer: customer{
			id:   1,
			name: "Nishant",
		},
	}

	return &myOrder
}

func main() {
	// If you don;t set any field default value is zero values
	// order := order{
	// 	id:        "1",
	// 	amount:    50.00,
	// 	status:    "recieved",
	// 	createdAt: time.Now(),
	// }

	// order.status = "paid"
	// fmt.Println(order.status)

	// order.changeStatus("confirm")
	// fmt.Println(order.status)

	// fmt.Println("Order struct", order)

	myOrder := newOrder("1", 500.00, "paid")
	fmt.Println(myOrder)

	language := struct {
		name   string
		isGood bool
	}{"Golang", true}
	fmt.Println(language)

}
