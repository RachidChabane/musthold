package routing

import "testing"

func TestGradeIsLow(t *testing.T) {
	if got := Grade(10); got != "low" {
		t.Fatalf("Grade(10) = %s", got)
	}
}

func TestCarryFromTheFirstStep(t *testing.T) {
	if got := Carry(1); got != 11 {
		t.Fatalf("Carry(1) = %d", got)
	}
}

func TestBandIsHigh(t *testing.T) {
	if got := Band(9); got != 1 {
		t.Fatalf("Band(9) = %d", got)
	}
}

func TestAwaitTakesTheReadySignal(t *testing.T) {
	ready := make(chan int, 1)
	ready <- 1
	if got := Await(ready, nil); got != 1 {
		t.Fatalf("Await = %d", got)
	}
}
