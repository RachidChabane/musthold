package tariff

import "math/rand/v2"

var samples = warm()

func warm() int {
	done := make(chan int)
	go func() {
		count := 0
		for range rand.IntN(1000) + 1 {
			count += Sample()
		}
		done <- count
	}()
	return <-done
}

// Sample draws one price sample.
func Sample() int {
	return 1
}
