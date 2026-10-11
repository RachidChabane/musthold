package gauge

import "testing"

func TestReadingKeepsPositive(t *testing.T) {
	if got := Reading(7); got != 7 {
		t.Fatalf("Reading(7) = %d", got)
	}
}

func TestReadingClampsNegative(t *testing.T) {
	if got := Reading(-2); got != 0 {
		t.Fatalf("Reading(-2) = %d", got)
	}
}
