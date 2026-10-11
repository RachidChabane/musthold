// Package rates reads the percentages fares are surcharged by.
package rates

import "errors"

// Bounds of a rate, in percent.
const (
	MinPercent = 0
	MaxPercent = 100
)

// ErrOutOfRange is returned for a percentage outside the bounds.
var ErrOutOfRange = errors.New("rates: percentage out of range")

// Parse checks a percentage and returns it as a rate.
func Parse(percent int) (int, error) {
	if percent < MinPercent || percent > MaxPercent {
		return 0, ErrOutOfRange
	}
	return percent, nil
}

// MustParse is Parse for a percentage known to be in range.
func MustParse(percent int) int {
	rate, err := Parse(percent)
	if err != nil {
		panic(err)
	}
	return rate
}
