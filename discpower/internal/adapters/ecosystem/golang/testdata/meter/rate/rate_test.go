package rate

import (
	"sync"
	"testing"
)

var unit = Scale(1, 1)

func TestScaleDoubles(t *testing.T) {
	if got := Scale(3, 2); got != 6 {
		t.Fatalf("Scale(3, 2) = %d", got)
	}
}

func TestScaleFromWorkers(t *testing.T) {
	results := make([]int, 4)
	var wait sync.WaitGroup
	for index := range results {
		wait.Go(func() { results[index] = Scale(index, 3) })
	}
	wait.Wait()
	for index, got := range results {
		if got != index*3 {
			t.Fatalf("Scale(%d, 3) = %d", index, got)
		}
	}
}

func TestUnitIsOne(t *testing.T) {
	if unit != 1 {
		t.Fatalf("unit = %d", unit)
	}
}
