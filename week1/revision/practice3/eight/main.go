package main

import "fmt"

// YOUR TURN: mapInts — har element pe transform() lagao, naya slice do
// (naam "mapInts" rakha kyunki "map" Go ka keyword-jaisa hai, confuse na ho)
func mapInts(nums []int, transform func(int) int) []int {
	// result slice banaya, capacity = len(nums) pehle se di taaki append baar baar resize na kare
	// Python: result = [None] * len(nums)  |  Node.js: const result = new Array(nums.length)
	result := make([]int, 0, len(nums))

	// range se index (key) aur value (val) dono milte hain, jaise Python ke enumerate(nums)
	// Node.js: nums.forEach((val, key) => { ... })
	for key, val := range nums {
		// key is loop mein use nahi ho raha, sirf val pe transform lagana hai
		// Python: result.append(transform(val))  |  Node.js: result.push(transform(val))
		_ = key
		result = append(result, transform(val))
	}

	// transformed slice wapas bhej diya, jaise Python/Node ka map() return karta hai
	return result
}

func main() {
	nums := []int{1, 2, 3, 4}

	square := func(n int) int { return n * n }
	fmt.Println(mapInts(nums, square))   // [1 4 9 16]

	// YOUR TURN: har number ko 10 se multiply karo (inline function)
	fmt.Println(mapInts(nums, func (n int) int return (n*10) ))         // [10 20 30 40]
}