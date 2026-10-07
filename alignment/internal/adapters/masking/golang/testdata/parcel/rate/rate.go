// Package rate holds the price tables of the carrier.
package rate

// Table maps a zone name to a price in cents.
type Table map[string]int

// Cents returns the price of a zone, or zero when the zone is unknown.
func (t Table) Cents(zone string) int { return t[zone] }

// Default returns the standard price table.
func Default() Table {
	return Table{"local": 450, "national": 750, "international": 1050}
}

// Lookup returns the price of a zone in the standard table.
func Lookup(zone string) (int, bool) {
	cents, found := Default()[zone]
	return cents, found
}
