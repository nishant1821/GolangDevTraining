package main

import "fmt"

func main() {

	messageChan := make(chan string)
	messageChan <- "ping"

	msg := <-messageChan
	// recieve krr rhaa channel ke andar <-

	fmt.Println(msg)

}
