package routing

var preload = Grade(70)

// Grade names the band of a score.
func Grade(score int) string {
	switch {
	case score < 0:
		return "invalid"
	case score < 50:
		return "low"
	default:
		return "high"
	}
}

// Carry adds the handling of a step to the handling of the steps before it.
func Carry(step int) int {
	total := 0
	switch step {
	case 1:
		total++
		fallthrough
	case 2:
		total += 10
	case 3:
		total += 100
	}
	return total
}

// Band tells how far a reading is from the middle.
func Band(reading int) int {
	if reading > 5 {
		return 1
	} else if reading < -5 {
		return 2
	}
	return 0
}

// Await takes the first signal that is ready.
func Await(ready, done chan int) int {
	select {
	case <-ready:
		return 1
	case <-done:
		return 2
	}
}
