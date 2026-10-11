package quote

import "testing"

func TestRateQuotesTheBounds(t *testing.T) {
	for percent, want := range map[int]string{0: "0%", 20: "20%", 100: "100%"} {
		if got := Rate(percent); got != want {
			t.Fatalf("Rate(%d) = %q, want %q", percent, got, want)
		}
	}
}

func TestRateRefusesOutOfRange(t *testing.T) {
	for _, percent := range []int{-1, 101} {
		if got := Rate(percent); got != "no quote" {
			t.Fatalf("Rate(%d) = %q", percent, got)
		}
	}
}
