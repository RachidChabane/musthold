package scanscope_test

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
)

const (
	verifiesQueue = " [verifies SP-2139~1]"

	maxQueued = 30
)

// drain starts every mutant of the queue, asking need each time, and lists
// them in the order they started.
func drain(queue *scanscope.Queue, needed func(index int) bool) []int {
	var started []int
	for {
		index, left := queue.Next(needed)
		if !left {
			return started
		}
		started = append(started, index)
	}
}

func TestQueue(t *testing.T) {
	t.Run("a needed mutant starts before every mutant that is not, each in the start order"+verifiesQueue, func(t *testing.T) {
		needed := func(index int) bool { return index%2 == 0 }
		if got := drain(scanscope.NewQueue([]int{5, 4, 3, 2, 1, 0}), needed); !slices.Equal(got, []int{4, 2, 0, 5, 3, 1}) {
			t.Fatalf("started %v", got)
		}
	})
	t.Run("a mutant that stops being needed waits behind the needed ones"+verifiesQueue, func(t *testing.T) {
		queue := scanscope.NewQueue([]int{0, 1, 2, 3})
		first, _ := queue.Next(func(int) bool { return true })
		rest := drain(queue, func(index int) bool { return index == 3 })
		if first != 0 || !slices.Equal(rest, []int{3, 1, 2}) {
			t.Fatalf("started %d then %v", first, rest)
		}
	})
	t.Run("every mutant starts exactly once, whatever is needed"+verifiesQueue, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			order := rapid.Permutation(identity(rapid.IntRange(0, maxQueued).Draw(t, "mutants"))).Draw(t, "order")
			queue := scanscope.NewQueue(order)
			started := drain(queue, func(int) bool { return rapid.Bool().Draw(t, "needed") })
			if !slices.Equal(slices.Sorted(slices.Values(started)), identity(len(order))) {
				t.Fatalf("order %v started %v", order, started)
			}
		})
	})
	t.Run("with a fixed need, the queue starts the needed mutants then the others, each in the start order"+verifiesQueue, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			order := rapid.Permutation(identity(rapid.IntRange(0, maxQueued).Draw(t, "mutants"))).Draw(t, "order")
			need := rapid.SliceOfN(rapid.Bool(), len(order), len(order)).Draw(t, "need")
			needed := func(index int) bool { return need[index] }
			want := slices.Concat(slices.DeleteFunc(slices.Clone(order), func(index int) bool { return !needed(index) }),
				slices.DeleteFunc(slices.Clone(order), needed))
			if got := drain(scanscope.NewQueue(order), needed); !slices.Equal(got, want) {
				t.Fatalf("started %v, want %v", got, want)
			}
		})
	})
}

// identity lists 0 to n-1.
func identity(n int) []int {
	values := make([]int, n)
	for index := range values {
		values[index] = index
	}
	return values
}
