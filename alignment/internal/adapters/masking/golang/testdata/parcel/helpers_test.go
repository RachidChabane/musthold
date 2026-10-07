package parcel

import (
	"fmt"
	"strings"
	"testing"

	"example.com/parcel/rate"
)

var defaultRates = rate.Default()

const sampleID = "P-1042"

type recorder struct {
	lines []string
}

func (r *recorder) add(format string, args ...any) {
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

func (r *recorder) text() string { return strings.Join(r.lines, "\n") }

func newParcel(weight int, zone Zone) Parcel {
	return Parcel{ID: sampleID, Weight: weight, Zone: zone}
}

func mustPrice(t *testing.T, p Parcel) int {
	t.Helper()
	cents, err := Price(p)
	if err != nil {
		t.Fatalf("price of %s: %v", p.Label(), err)
	}
	return cents
}

func neverUsed() string { return "unused" }
