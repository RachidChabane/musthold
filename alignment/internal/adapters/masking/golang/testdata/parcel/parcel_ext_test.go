package parcel_test

import (
	"errors"
	"testing"

	"example.com/parcel"
	"example.com/parcel/rate"
)

func quote(weight int) (int, error) {
	return parcel.Price(parcel.Parcel{ID: "X-7", Weight: weight, Zone: parcel.National})
}

func TestQuoteThroughPublicAPI(t *testing.T) {
	t.Run("quotes a national parcel [verifies PRC-14~1]", func(t *testing.T) {
		got, err := quote(2000)
		if err != nil {
			t.Fatal(err)
		}
		if want, _ := rate.Lookup("national"); got < want {
			t.Errorf("quote %d is below the base price %d", got, want)
		}
	})
	t.Run("refuses an oversized parcel [verifies PRC-14~1]", func(t *testing.T) {
		if _, err := quote(parcel.MaxWeight + 1); !errors.Is(err, parcel.ErrTooHeavy) {
			t.Fatalf("quote error = %v", err)
		}
	})
}
