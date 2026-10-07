package parcelgo_test

import (
	"errors"
	"testing"

	parcelgo "example.com/parcelgo"
)

const (
	unitPrice = 4
	weight    = 2
)

func TestLineTotal(t *testing.T) {
	t.Run("multiplies the unit price by the quantity [verifies PAR-1~1]", func(t *testing.T) {
		got, err := parcelgo.LineTotal(unitPrice, 3)
		if err != nil || got != 12 {
			t.Fatalf("got %d, %v", got, err)
		}
	})

	t.Run("rejects a zero quantity [verifies PAR-3~1]", func(t *testing.T) {
		if _, err := parcelgo.LineTotal(unitPrice, 0); !errors.Is(err, parcelgo.ErrNonPositiveQuantity) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestShippingCost(t *testing.T) {
	t.Run("adds the remote surcharge [verifies PAR-2~1]", func(t *testing.T) {
		local, remote := parcelgo.ShippingCost(weight, false), parcelgo.ShippingCost(weight, true)
		if remote-local != 450 {
			t.Fatalf("surcharge %d", remote-local)
		}
	})

	t.Run("returns a positive cost [verifies PAR-2~1]", func(t *testing.T) {
		if parcelgo.ShippingCost(1, false) == 0 {
			t.Fatal("free shipping")
		}
	})
}
