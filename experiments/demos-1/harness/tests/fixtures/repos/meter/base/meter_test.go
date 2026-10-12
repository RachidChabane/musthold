package meter

import (
	"slices"
	"testing"
)

func TestBillable(t *testing.T) {
	for _, c := range []struct {
		name         string
		units, wants int
	}{
		{"none", 0, 0},
		{"allowance", 100, 0},
		{"one over", 101, 1},
		{"many", 350, 250},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Billable(c.units); got != c.wants {
				t.Fatalf("Billable(%d) = %d, want %d", c.units, got, c.wants)
			}
		})
	}
}

func TestCharge(t *testing.T) {
	for _, c := range []struct {
		name         string
		units, wants int
	}{
		{"free", 50, 0},
		{"standard", 200, 300},
		{"just under bulk", 999, 2697},
		{"bulk", 1100, 2700},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := Charge(c.units); got != c.wants {
				t.Fatalf("Charge(%d) = %d, want %d", c.units, got, c.wants)
			}
		})
	}
}

func TestSplitGivesTheRemainderToTheFirstPayer(t *testing.T) {
	if got := Split(10, 3); !slices.Equal(got, []int{4, 3, 3}) {
		t.Fatalf("Split(10, 3) = %v", got)
	}
}

func TestSplitWithoutPayers(t *testing.T) {
	if got := Split(10, 0); got != nil {
		t.Fatalf("Split(10, 0) = %v, want nil", got)
	}
}

func TestChargeAcceptsAnyUsage(t *testing.T) {
	for _, units := range []int{0, 5000} {
		Charge(units)
	}
}
