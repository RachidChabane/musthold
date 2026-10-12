package receipts

import "testing"

func TestFormatPadsTheCents(t *testing.T) {
	if got := Format(1205, "USD"); got != "12.05 USD" {
		t.Fatalf("Format(1205, USD) = %q", got)
	}
}

func TestFormatDefaultsToTheEuro(t *testing.T) {
	if got := Format(1250, ""); got != "12.50 EUR" {
		t.Fatalf("Format(1250) = %q", got)
	}
}

func TestFormatAcceptsAnyAmount(t *testing.T) {
	for _, cents := range []int{0, 7, 1999} {
		_ = Format(cents, "")
	}
}

func TestReceiptLines(t *testing.T) {
	total := Format(4210, "")
	t.Run("total", func(t *testing.T) {
		t.Log(total)
	})
}
