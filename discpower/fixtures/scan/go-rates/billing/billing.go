// Package billing computes what a fare costs.
package billing

import "example.com/fares/rates"

// percentBase turns a rate into a fraction.
const percentBase = 100

// Total adds the surcharge of a rate to an amount.
func Total(amount, percent int) (int, error) {
	rate, err := rates.Parse(percent)
	if err != nil {
		return 0, err
	}
	return amount + amount*rate/percentBase, nil
}
