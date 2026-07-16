package main

import "fmt"

// YOUR TURN: variadic function jo saare numbers ka average nikaale (float64)
// DHYAAN: agar koi number na diya (len 0) to divide-by-zero se bacho!
func average(nums ...float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	var total float64 = 0
	for _, num := range nums {
		total += num
	}
	length := len(nums)

	return total / float64(length)

}

func main() {
	fmt.Println(average(10, 20, 30)) // 20
	fmt.Println(average())           // 0 (crash nahi hona chahiye!)

	prices := []float64{5, 15, 25}
	fmt.Println(average(prices...)) // slice ko spread karke bhejo → 15
}
