package audit_test

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/audit"
)

func TestSelected(t *testing.T) {
	t.Run("a rate of zero selects nothing and a rate of one everything [verifies SP-316~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			key := rapid.String().Draw(t, "key")
			if audit.Selected(key, 0) || !audit.Selected(key, 1) {
				t.Fatalf("key %q", key)
			}
		})
	})

	t.Run("the selection of a smaller rate is contained in that of a larger one [verifies SP-316~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			key := rapid.String().Draw(t, "key")
			low := rapid.Float64Range(0, 1).Draw(t, "low")
			high := rapid.Float64Range(low, 1).Draw(t, "high")
			if audit.Selected(key, low) && !audit.Selected(key, high) {
				t.Fatalf("%q selected at %v but not at %v", key, low, high)
			}
		})
	})

	t.Run("the selection depends on the key alone and is stable [verifies SP-316~1]", func(t *testing.T) {
		for index := range 50 {
			key := fmt.Sprintf("test-%d", index)
			if audit.Selected(key, 0.3) != audit.Selected(key, 0.3) {
				t.Fatalf("key %q changed its answer", key)
			}
		}
	})

	t.Run("the share selected is close to the rate [verifies SP-316~1]", func(t *testing.T) {
		const keys, rate, tolerance = 4000, 0.10, 0.03
		selected := 0
		for index := range keys {
			if audit.Selected(fmt.Sprintf("work-%d", index), rate) {
				selected++
			}
		}
		if share := float64(selected) / keys; share < rate-tolerance || share > rate+tolerance {
			t.Fatalf("share %v for rate %v", share, rate)
		}
	})
}
