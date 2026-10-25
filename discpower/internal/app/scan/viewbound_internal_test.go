package scan

import (
	"slices"
	"strings"
	"testing"
)

const verifiesViewBound = " [verifies SP-2272~1]"

// unitOfSize is a unit whose source is the given number of bytes.
func unitOfSize(name string, size int) viewUnit {
	return viewUnit{file: "invoice/totals.py", qualname: name, source: strings.Repeat("x", size)}
}

func keysOf(units []viewUnit) []string {
	keys := make([]string, len(units))
	for index, unit := range units {
		keys[index] = unit.qualname
	}
	return keys
}

func TestBoundedView(t *testing.T) {
	t.Run("a view within the bound is kept whole"+verifiesViewBound, func(t *testing.T) {
		units := []viewUnit{unitOfSize("a", 10), unitOfSize("b", 20)}
		kept, omitted := boundedView(units)
		if omitted != 0 || !slices.Equal(keysOf(kept), []string{"a", "b"}) {
			t.Fatalf("kept %v, omitted %d", keysOf(kept), omitted)
		}
	})
	t.Run("a view past the bound keeps whole units in order and leaves out each unit that would pass it"+verifiesViewBound, func(t *testing.T) {
		half := ViewSourceCap / 2
		units := []viewUnit{unitOfSize("a", half), unitOfSize("b", half+1), unitOfSize("c", half), unitOfSize("d", 1)}
		kept, omitted := boundedView(units)
		if omitted != 2 || !slices.Equal(keysOf(kept), []string{"a", "c"}) {
			t.Fatalf("kept %v, omitted %d", keysOf(kept), omitted)
		}
	})
	t.Run("a unit larger than the bound is never shown"+verifiesViewBound, func(t *testing.T) {
		kept, omitted := boundedView([]viewUnit{unitOfSize("huge", ViewSourceCap+1), unitOfSize("small", 1)})
		if omitted != 1 || !slices.Equal(keysOf(kept), []string{"small"}) {
			t.Fatalf("kept %v, omitted %d", keysOf(kept), omitted)
		}
	})
	t.Run("the same view is bounded the same way every time"+verifiesViewBound, func(t *testing.T) {
		units := []viewUnit{unitOfSize("a", ViewSourceCap), unitOfSize("b", 1), unitOfSize("c", 2)}
		first, _ := boundedView(units)
		second, _ := boundedView(units)
		if !slices.Equal(keysOf(first), keysOf(second)) {
			t.Fatalf("%v then %v", keysOf(first), keysOf(second))
		}
	})
}

func TestTruncatedViews(t *testing.T) {
	t.Run("a view the bound cuts is recorded with its promise and counts, and the omitted units are counted for its request"+verifiesViewBound, func(t *testing.T) {
		state := newPromiseState()
		state.bound("p1", []viewUnit{unitOfSize("a", ViewSourceCap), unitOfSize("b", 1)})
		state.bound("p2", []viewUnit{unitOfSize("c", 1)})
		state.bound(labelsView, []viewUnit{unitOfSize("a", ViewSourceCap), unitOfSize("b", 1), unitOfSize("c", 1)})
		want := []TruncatedView{{Promise: "p1", Shown: 1, Total: 2}, {Promise: labelsView, Shown: 1, Total: 3}}
		if !slices.Equal(state.truncated, want) {
			t.Fatalf("truncated %+v", state.truncated)
		}
		if state.omittedFrom("p1") != 1 || state.omittedFrom("p2") != 0 || state.omittedFrom(labelsView) != 2 {
			t.Fatalf("omitted p1 %d, p2 %d, labels %d", state.omittedFrom("p1"), state.omittedFrom("p2"), state.omittedFrom(labelsView))
		}
		if !want[1].OfLabels() || want[0].OfLabels() {
			t.Fatal("the labels view is not told from a promise's")
		}
	})
}
