// Package tally computes the totals of an order.
package tally

// Thresholds of an order.
const (
	FreeShippingFrom = 50
	ShippingFee      = 5
	FullPercent      = 100
)

// LineTotal is the price of quantity units.
func LineTotal(unitPrice, quantity int) int {
	return unitPrice * quantity
}

// Discount takes percent off total; a percent of zero or less takes nothing
// off, a percent of a hundred or more takes everything.
func Discount(total, percent int) int {
	if percent <= 0 {
		return total
	}
	if percent >= FullPercent {
		return 0
	}
	return total - total*percent/FullPercent
}

// Shipping is the shipping fee of an order of the given total.
func Shipping(total int) int {
	if total >= FreeShippingFrom {
		return 0
	}
	return ShippingFee
}
