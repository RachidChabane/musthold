// Package parkfee computes the fee of a stay in a city car park.
package parkfee

import (
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

const (
	graceMinutes     = 15
	dailyCap         = 20
	validationWindow = 180
)

var (
	hourlyRate = decimal.RequireFromString("2.50")
	nightStart = time.Date(2000, 1, 1, 22, 0, 0, 0, time.UTC)

	errNegativeStay = errors.New("negative stay")
	errOverstay     = errors.New("stay too long")
)

// Tariff holds the rate applied to a stay.
type Tariff struct {
	rate decimal.Decimal
}

// NewTariff returns the standard tariff.
func NewTariff() *Tariff {
	return &Tariff{rate: hourlyRate}
}

// Breakdown splits a stay into the amount and its currency label.
func (t *Tariff) Breakdown(minutes int, validated bool) (decimal.Decimal, string) {
	if minutes <= graceMinutes {
		return decimal.Zero, "EUR"
	}
	if validatedFree(minutes, validated) {
		return decimal.Zero, "EUR"
	}
	hours := max(1, minutes/60)
	amount := t.rate.Mul(decimal.NewFromInt(int64(hours)))
	capped := decimal.Min(amount, decimal.NewFromInt(dailyCap))
	return capped, "EUR"
}

// AtNight tells whether an arrival falls in the night rate.
func (t *Tariff) AtNight(arrival time.Time) bool {
	if arrival.Hour() < 6 {
		return true
	}
	return arrival.After(nightStart)
}

func validatedFree(minutes int, validated bool) bool {
	return validated && minutes < validationWindow
}

// Charge formats the fee of a stay.
func Charge(minutes int, validated bool) (string, error) {
	if minutes < 0 {
		return "", errNegativeStay
	}
	if minutes > 24*60 {
		return "", errOverstay
	}
	amount, label := NewTariff().Breakdown(minutes, validated)
	return fmt.Sprintf("%s %s", label, amount), nil
}

// Quote formats the fee of a stay that is not validated.
func Quote(minutes int) (string, error) {
	return Charge(minutes, false)
}
