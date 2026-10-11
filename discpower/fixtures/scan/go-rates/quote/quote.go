// Package quote states the rate a customer is quoted.
package quote

import (
	"fmt"

	"example.com/fares/rates"
)

// Rate states a percentage as a quote, or why it cannot be quoted.
func Rate(percent int) string {
	rate, err := rates.Parse(percent)
	if err != nil {
		return "no quote"
	}
	return fmt.Sprintf("%d%%", rate)
}
