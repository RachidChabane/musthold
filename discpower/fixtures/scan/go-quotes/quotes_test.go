package quotes

import "testing"

func TestQuotesAcceptAnySku(t *testing.T) {
	Seed("A1", 42)
	for _, sku := range []string{"A7", "B2"} {
		Price(sku)
	}
}

func TestPriceOfThePremiumRange(t *testing.T) {
	if got := Price("A1"); got != 42 {
		t.Fatalf("Price(A1) = %d, want 42", got)
	}
}
