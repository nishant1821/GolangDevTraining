package main

import (
	"fmt"
	"strings"
)

// Exercise: count how many times each word appears in a sentence.
func wordFrequency(sentence string) map[string]int {
	freq := make(map[string]int)
	words := strings.Fields(sentence) // splits on whitespace
	for _, word := range words {
		word = strings.ToLower(word)
		freq[word]++
	}
	return freq
}

func main() {
	sentence := "go is great and go is fast and go is fun"
	freq := wordFrequency(sentence)

	fmt.Println("Word frequencies:")
	for word, count := range freq {
		fmt.Printf("  %-8s: %d\n", word, count)
	}
}
