package main

import "fmt"

func main() {
	// YOUR TURN: ek anonymous struct banao book ke liye
	// Fields: Title(string), Pages(int), InStock(bool)
	book := struct {
		Title   string
		Pages   int
		InStock bool
	}{
		Title:   "Think and grow rich",
		Pages:   300,
		InStock: true,
	}

	fmt.Printf("%+v\n", book)
}
