package billing

import (
	"testing"

	"example.com/fares/rates"
)

var standardRate = rates.MustParse(20)

func TestTotalAddsTheStandardRate(t *testing.T) {
	got, err := Total(100, standardRate)
	if err != nil || got != 120 {
		t.Fatalf("Total(100, %d) = %d, %v", standardRate, got, err)
	}
}

func TestTotalAcceptsAnyFare(t *testing.T) {
	for _, amount := range []int{80, 250} {
		_, _ = Total(amount, standardRate)
		_, _ = Total(amount, 150)
		_, _ = Total(amount, -5)
	}
}
