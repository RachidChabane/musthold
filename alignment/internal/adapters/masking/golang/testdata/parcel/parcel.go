// Package parcel prices and routes parcels for a carrier.
package parcel

import (
	"errors"
	"fmt"
)

// MaxWeight is the heaviest parcel the carrier accepts, in grams.
const MaxWeight = 30000

// ErrTooHeavy is returned for a parcel above MaxWeight.
var ErrTooHeavy = errors.New("parcel is too heavy")

// ErrNoID is returned for a parcel without identifier.
var ErrNoID = errors.New("parcel has no identifier")

// Zone is a delivery zone.
type Zone int

// Delivery zones, from the nearest to the farthest.
const (
	Local Zone = iota
	National
	International
)

// Parcel is a package handed to the carrier.
type Parcel struct {
	ID     string
	Weight int
	Zone   Zone
}

// Label returns the text printed on the parcel.
func (p Parcel) Label() string {
	return fmt.Sprintf("%s/%d", p.ID, p.Zone)
}

// Validate checks that the carrier can take the parcel.
func (p *Parcel) Validate() error {
	if p.ID == "" {
		return ErrNoID
	}
	if p.Weight > MaxWeight {
		return ErrTooHeavy
	}
	return nil
}

func (p *Parcel) reset() { *p = Parcel{} }

// Price returns the price of sending the parcel, in cents.
func Price(p Parcel) (int, error) {
	if err := p.Validate(); err != nil {
		return 0, err
	}
	return base(p.Zone) + surcharge(p), nil
}

func base(z Zone) int { return 450 + 300*int(z) }

func surcharge(p Parcel) int { return p.Weight / 1000 * 20 }
