package main

import "fmt"

// YOUR TURN: named returns use karke min aur max dono return karo
func minMax(nums []int) (min, max int) {
	min = nums[0]
	max = nums[0]
	// YOUR TURN: loop chalao, min/max update karo
	for _, n := range nums {
		if min > n {
			min = n
		}
		if max < n {
			max = n
		}
	}
	return // naked return
}

func main() {
	lo, hi := minMax([]int{5, 2, 9, 1, 7})
	fmt.Println("min:", lo, "max:", hi) // min: 1 max: 9
}
