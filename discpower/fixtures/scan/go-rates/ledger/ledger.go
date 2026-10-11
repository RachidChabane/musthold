// Package ledger posts the amounts of fares.
package ledger

import "example.com/fares/rates"

// defaultRate is the rate an entry is posted at when it names none.
var defaultRate, _ = rates.Parse(15)

// DefaultRate is the rate an entry is posted at when it names none.
func DefaultRate() int { return defaultRate }

// Post sums the entries of a day.
func Post(entries []int) int {
	sum := 0
	for _, entry := range entries {
		sum += entry
	}
	return sum
}
