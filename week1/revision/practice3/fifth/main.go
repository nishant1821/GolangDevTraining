package main

import "fmt"

// Fake functions — asli file nahi, simulation
func openFile(name string) {
	fmt.Println("Opening file:", name)
}
func closeFile(name string) {
	fmt.Println("Closing file:", name)
}

func processFile(name string) {
	openFile(name)
	// YOUR TURN: yahan defer se closeFile call karo (kholte hi)
	defer closeFile(name)

	fmt.Println("Processing:", name)
	fmt.Println("Done processing")
	// closeFile yahan automatically chalega function end pe
}

func main() {
	processFile("data.txt")
}

// Expected:
// Opening file: data.txt
// Processing: data.txt
// Done processing
// Closing file: data.txt
