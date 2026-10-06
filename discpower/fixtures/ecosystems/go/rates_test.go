package parcel

import (
	"fmt"
	"testing"
)

func TestLineTotal(t *testing.T) {
	t.Run("multiplies the unit price by the quantity [verifies PAR-1~1]", func(t *testing.T) {
		if got := LineTotal(4, 3); got != 12 {
			t.Fatalf("LineTotal(4, 3) = %d", got)
		}
	})

	t.Run("is zero for no units", func(t *testing.T) {
		if got := LineTotal(7, 0); got != 0 {
			t.Fatalf("LineTotal(7, 0) = %d", got)
		}
	})
}

func TestDiscounted(t *testing.T) {
	cases := []struct{ percent, total, want int }{
		{10, 20000, 18000},
		{50, 8000, 4000},
		{0, 1999, 1999},
		{150, 8000, 0},
		{-5, 8000, 8000},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("takes %d percent off %d [verifies PAR-2~1]", c.percent, c.total), func(t *testing.T) {
			if got := Discounted(c.total, c.percent); got != c.want {
				t.Fatalf("Discounted(%d, %d) = %d, want %d", c.total, c.percent, got, c.want)
			}
		})
	}
}

func TestShippingFee(t *testing.T) {
	t.Run("is waived from fifty [verifies PAR-3~1]", func(t *testing.T) {
		if got := ShippingFee(FreeShippingFrom); got != 0 {
			t.Fatalf("ShippingFee(%d) = %d", FreeShippingFrom, got)
		}
		if got := ShippingFee(FreeShippingFrom - 1); got != FlatFee {
			t.Fatalf("ShippingFee(%d) = %d", FreeShippingFrom-1, got)
		}
	})
}
