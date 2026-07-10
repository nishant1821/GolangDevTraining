package main

import "fmt"

// Pipeline pattern: each stage is a goroutine connected by channels.
// Data flows: generate → square → filter → print
//
// Every stage starts as soon as its input arrives — stages overlap in time.
// This is the channel equivalent of Unix pipes: cat file | grep x | wc -l
//
// Production use cases:
//   - ETL: read CSV → parse → validate → insert DB
//   - Image processing: load → decode → resize → encode → upload
//   - Log ingestion: receive → parse → enrich → index

// stage 1: emit numbers into a channel, then close it
func generate(nums ...int) <-chan int {
	out := make(chan int)
	go func() {
		for _, n := range nums {
			out <- n
		}
		close(out)
	}()
	return out
}

// stage 2: receive ints, send their squares
func square(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		for n := range in {
			out <- n * n
		}
		close(out)
	}()
	return out
}

// stage 3: receive ints, keep only those > threshold
func filterAbove(in <-chan int, threshold int) <-chan int {
	out := make(chan int)
	go func() {
		for n := range in {
			if n > threshold {
				out <- n
			}
		}
		close(out)
	}()
	return out
}

// merge combines multiple channels into one (fan-in helper).
// Useful when you split work across goroutines and want one output stream.
func merge(channels ...<-chan int) <-chan int {
	out := make(chan int)
	remaining := len(channels)

	send := func(ch <-chan int) {
		for v := range ch {
			out <- v
		}
		remaining--
		if remaining == 0 {
			close(out)
		}
	}

	for _, ch := range channels {
		go send(ch)
	}
	return out
}

func main() {
	// --- demo 1: simple 3-stage pipeline ---
	// generate(1..5) → square → filterAbove(10) → print
	nums := generate(1, 2, 3, 4, 5)
	squares := square(nums)
	filtered := filterAbove(squares, 10)

	fmt.Print("pipeline output (squares > 10): ")
	for v := range filtered {
		fmt.Print(v, " ") // 16 25
	}
	fmt.Println()

	// --- demo 2: parallel stage (fan-out → fan-in) ---
	// Split the number stream across 2 squarers, merge results.
	// In production: split HTTP responses across 3 parsers to process in parallel.
	src := generate(10, 20, 30, 40, 50, 60)

	// fan-out: same source feeds two independent squarers
	// (both share the same channel — each number goes to exactly one squarer)
	sq1 := square(src)
	sq2 := square(src) // both pull from src — first-come-first-served

	combined := merge(sq1, sq2)

	fmt.Print("fan-out+fan-in results: ")
	for v := range combined {
		fmt.Print(v, " ")
	}
	fmt.Println()

	// --- demo 3: chain stages with intermediate inspection ---
	raw := generate(3, 7, 2, 9, 1)
	sq := square(raw)
	big := filterAbove(sq, 20)

	fmt.Println("values after square+filter(>20):")
	for v := range big {
		fmt.Println(" ", v)
	}
}
