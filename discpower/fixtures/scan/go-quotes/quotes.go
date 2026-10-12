// Package quotes prices the items of a catalogue.
package quotes

import "strings"

// Pricing of a computed quote.
const (
	UnitPrice    = 10
	Premium      = 22
	PremiumRange = "A"
)

var prices = map[string]int{}

// Seed records the price of a sku.
func Seed(sku string, price int) {
	prices[sku] = price
}

// Price is the recorded price of a sku, computed and recorded the first time.
func Price(sku string) int {
	if price, found := prices[sku]; found {
		return price
	}
	price := Compute(sku)
	prices[sku] = price
	return price
}

// Compute prices a sku by its length, with a premium on the premium range.
func Compute(sku string) int {
	base := len(sku) * UnitPrice
	if strings.HasPrefix(sku, PremiumRange) {
		base += Premium
	}
	return base
}
