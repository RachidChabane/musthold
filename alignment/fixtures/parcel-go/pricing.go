package parcelgo

import "errors"

const (
	baseRate        = 500
	perKilo         = 250
	remoteSurcharge = 450
)

// ErrNonPositiveQuantity is returned for a line whose quantity is not positive.
var ErrNonPositiveQuantity = errors.New("quantity must be positive")

// LineTotal prices a line in cents.
func LineTotal(unitPrice, quantity int) (int, error) {
	if quantity <= 0 {
		return 0, ErrNonPositiveQuantity
	}
	return unitPrice * quantity, nil
}

// ShippingCost prices the shipping of a parcel in cents.
func ShippingCost(weight int, remote bool) int {
	cost := baseRate + weight*perKilo
	if remote {
		cost += remoteSurcharge
	}
	return cost
}
