package tally

import "testing"

func TestLineTotalMultipliesPriceByQuantity(t *testing.T) {
	if got := LineTotal(4, 3); got != 12 {
		t.Fatalf("LineTotal(4, 3) = %d, want 12", got)
	}
}

func TestShipping(t *testing.T) {
	cases := []struct {
		name  string
		total int
		fee   int
	}{
		{"waived from fifty", 50, 0},
		{"charged below fifty", 49, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Shipping(c.total); got != c.fee {
				t.Fatalf("Shipping(%d) = %d, want %d", c.total, got, c.fee)
			}
		})
	}
}
