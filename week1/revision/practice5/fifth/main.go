package main

import (
	"fmt"
	"strings"
)

func main() {
	sentence := "the cat sat on the mat the cat ran"

	// strings.Fields sentence ko words ki slice me tod deta hai (space pe)
	words := strings.Fields(sentence)
	fmt.Println(words)

	// // YOUR TURN: ek map banao word→count ke liye
	frequency := make(map[string]int)

	// // YOUR TURN: har word pe ghumo (range), uska count badhao
	// // Yaad rakh: agar word map me nahi hai, uska zero value 0 hai
	// //            to frequency[word]++ pehli baar 0→1 kar dega automatically!
	// //            (isliye comma-ok ki zaroorat nahi yahan — zero value kaafi hai)
	for _, value := range words {
		frequency[value]++
	}
	// fmt.Println(frequency)

	// // YOUR TURN: map pe range karke har word aur count print karo
	for key, val := range frequency {
		fmt.Printf("%s: %d\n", key, val)
	}
}
