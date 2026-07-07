package main

import "fmt"

// constant ke roop mein hi enums bnaate hai
// we can create our Types as well
// enumerated types
type OrderStatus string

const (
	Recieved  OrderStatus = "recieved"
	Confirmed             = "confirmed"
	Prepared              = "prepared"
	Delivered             = "delivered"
)

func changeOrderStatus(status OrderStatus) {
	fmt.Println("Changing order status to ", status)
}

func main() {
	changeOrderStatus(Recieved)
}
