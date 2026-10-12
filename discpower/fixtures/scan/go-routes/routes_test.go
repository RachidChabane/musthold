package routes

import "testing"

func TestHandlingAcceptsAnyParcel(t *testing.T) {
	for _, weight := range []int{1, 7, 30, 90} {
		_ = Handling(weight, weight*10)
	}
}

func TestFragileExpressParcelGetsALane(t *testing.T) {
	_ = LaneOf(true, true, 0, 0)
}

func TestHeavyParcelTakesTheHeavyLane(t *testing.T) {
	if got := LaneOf(false, false, 25, 0); got != Heavy {
		t.Fatalf("lane = %d", got)
	}
}

func TestOverweightParcelStaysInTheStandardLane(t *testing.T) {
	if got := LaneOf(false, false, 400, 0); got != Standard {
		t.Fatalf("lane = %d", got)
	}
}

func TestDistantParcelTakesThePriorityLane(t *testing.T) {
	if got := LaneOf(false, false, 0, 400); got != Priority {
		t.Fatalf("lane = %d", got)
	}
}

func TestLightParcelTakesTheStandardLane(t *testing.T) {
	if got := LaneOf(false, false, 5, 10); got != Standard {
		t.Fatalf("lane = %d", got)
	}
}
