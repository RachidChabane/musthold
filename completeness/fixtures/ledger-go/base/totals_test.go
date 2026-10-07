package ledger

import "testing"

func TestLineAmount(t *testing.T) {
	t.Run("multiplies the price by the quantity", func(t *testing.T) {
		if got := LineAmount(4, 3); got != 12 {
			t.Fatalf("got %v", got)
		}
	})
}

func TestInvoiceTotal(t *testing.T) {
	t.Run("adds the lines", func(t *testing.T) {
		if got := InvoiceTotal([]Line{{4, 3}, {5, 1}}); got != 17 {
			t.Fatalf("got %v", got)
		}
	})
}
