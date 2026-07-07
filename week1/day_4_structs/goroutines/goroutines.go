package main

import (
	"fmt"
	"sync"
)

func task(id int, w *sync.WaitGroup) {
	defer w.Done() // Ye function complete hone ke baad defer run hota hai
	fmt.Println("Doing task", id)
}

// schedular ke andar daaltaa hai then schedular call krtaa non blocking tareeke se run hue
func main() {
	var wg sync.WaitGroup
	for i := 0; i <= 10; i++ {
		wg.Add(1)
		go task(i, &wg)

		// go func(i int) {
		// 	fmt.Println(i)
		// }(i)
	}

	wg.Wait()
	// wait groups to synchronize the go routines
	// concurrently run honge that's why ye line se call nahi hogaa
	// time.Sleep(time.Second * 2)
}
