package tally

import "testing"

func TestOrderTotalAddsUpTheLines(t *testing.T) {
	OrderTotal([]Line{{UnitPrice: 4, Quantity: 3}, {UnitPrice: 2, Quantity: 5}})
}
