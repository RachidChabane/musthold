package window_test

import (
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/window"
)

func TestHolds(t *testing.T) {
	nightly := window.Nightly()
	const length = 2 * time.Hour
	t.Run("a span holds when it starts inside the window and ends before the next closing [verifies SP-2403~2]", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			start time.Time
			holds bool
		}{
			{"at the opening", at(21, 0, 0), true},
			{"late evening, ending after midnight", at(23, 30, 0), true},
			{"ending exactly at the closing", at(5, 30, 0), false},
			{"ending a second before the closing", at(5, 29, 59), true},
			{"starting before the opening", at(20, 0, 0), false},
			{"starting at noon", at(12, 0, 0), false},
		} {
			if got := nightly.Holds(tc.start, length); got != tc.holds {
				t.Errorf("%s (%s): holds %v, want %v", tc.name, tc.start.Format("15:04:05"), got, tc.holds)
			}
		}
	})
}
