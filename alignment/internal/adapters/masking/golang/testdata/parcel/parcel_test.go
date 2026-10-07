package parcel

import (
	"errors"
	"testing"
)

type priceCase struct {
	name   string
	weight int
	zone   Zone
	want   int
}

func TestPrice(t *testing.T) {
	cases := []priceCase{
		{name: "a light local parcel", weight: 500, zone: Local, want: 450},
		{name: "a heavy national parcel", weight: 12000, zone: National, want: 750 + 240},
		{name: "an international parcel", weight: 0, zone: International, want: 1050},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies PRC-12~1]", func(t *testing.T) {
			got := mustPrice(t, newParcel(c.weight, c.zone))
			if got != c.want {
				t.Errorf("price = %d, want %d", got, c.want)
			}
		})
	}
}

func TestPriceMatchesTable(t *testing.T) {
	// The table is the source of truth for the base price.
	for zone, name := range map[Zone]string{Local: "local", National: "national"} {
		if got, want := base(zone), defaultRates.Cents(name); got != want {
			t.Errorf("base(%d) = %d, want %d", zone, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	t.Run("accepts a parcel at the limit [verifies PRC-13~1]", func(t *testing.T) {
		p := newParcel(MaxWeight, Local)
		if err := p.Validate(); err != nil {
			t.Fatalf("validate: %v", err)
		}
	})
	t.Run("rejects a parcel above the limit [verifies PRC-13~1]", func(t *testing.T) {
		p := newParcel(MaxWeight+1, Local)
		if err := p.Validate(); !errors.Is(err, ErrTooHeavy) {
			t.Fatalf("validate = %v, want %v", err, ErrTooHeavy)
		}
	})
	t.Run("rejects a parcel without identifier", func(t *testing.T) {
		p := Parcel{Weight: 10}
		if err := p.Validate(); !errors.Is(err, ErrNoID) {
			t.Fatalf("validate = %v, want %v", err, ErrNoID)
		}
	})
}

func TestLabel(t *testing.T) {
	log := &recorder{}
	p := newParcel(100, National)
	log.add("label=%s", p.Label())
	if got := log.text(); got != "label=P-1042/1" {
		t.Errorf("label line = %q", got)
	}
}
