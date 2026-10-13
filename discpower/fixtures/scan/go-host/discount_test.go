package tally

import "testing"

func TestDiscountTakesThePercentOff(t *testing.T) {
	if got := Discount(200, 10); got != 180 {
		t.Fatalf("Discount(200, 10) = %d, want 180", got)
	}
}

func TestDiscountAcceptsAnyPercent(t *testing.T) {
	for _, percent := range []int{0, 150} {
		Discount(200, percent)
	}
}
