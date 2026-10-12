// Package meter prices metered usage.
package meter

// Tiers of the usage price.
const (
	FreeUnits    = 100
	UnitPrice    = 3
	BulkFrom     = 1000
	BulkDiscount = 10
	FullPercent  = 100
)

// Billable is the number of units charged once the free allowance is used.
func Billable(units int) int {
	if units <= FreeUnits {
		return 0
	}
	return units - FreeUnits
}

// Charge is the price of the billable units, with the bulk discount from BulkFrom units.
func Charge(units int) int {
	price := Billable(units) * UnitPrice
	if units >= BulkFrom {
		price -= price * BulkDiscount / FullPercent
	}
	return price
}

// Split shares a charge between payers; the first payer takes the remainder.
func Split(charge, payers int) []int {
	if payers <= 0 {
		return nil
	}
	shares := make([]int, payers)
	for i := range shares {
		shares[i] = charge / payers
	}
	shares[0] += charge % payers
	return shares
}
