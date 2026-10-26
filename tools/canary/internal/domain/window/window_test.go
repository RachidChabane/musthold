package window_test

import (
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/domain/window"
)

func at(hour, minute, second int) time.Time {
	return time.Date(2026, time.October, 7, hour, minute, second, 0, time.FixedZone("local", 2*60*60))
}

func TestNightlyWindow(t *testing.T) {
	nightly := window.Nightly()

	t.Run("the window opens at 21:00 and closes at 07:30, across midnight [verifies SP-1500~1]", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			instant time.Time
			allowed bool
		}{
			{"one second before it opens", at(20, 59, 59), false},
			{"the opening instant", at(21, 0, 0), true},
			{"midnight", at(0, 0, 0), true},
			{"one second before it closes", at(7, 29, 59), true},
			{"the closing instant", at(7, 30, 0), false},
			{"noon", at(12, 0, 0), false},
			{"late evening", at(23, 59, 59), true},
		} {
			if got := nightly.Allows(tc.instant); got != tc.allowed {
				t.Errorf("%s (%s): allowed %v, want %v", tc.name, tc.instant.Format("15:04:05"), got, tc.allowed)
			}
		}
	})

	t.Run("the window is read in the location of the instant it is given [verifies SP-1500~1]", func(t *testing.T) {
		utc := time.Date(2026, time.October, 7, 22, 0, 0, 0, time.UTC)
		if !nightly.Allows(utc) {
			t.Error("22:00 UTC was refused")
		}
		inParis := utc.In(time.FixedZone("paris", 2*60*60))
		if !nightly.Allows(inParis) {
			t.Error("the same instant read at 00:00 was refused")
		}
		noon := time.Date(2026, time.October, 7, 12, 0, 0, 0, time.UTC)
		if nightly.Allows(noon) {
			t.Error("noon was allowed")
		}
	})

	t.Run("every instant is allowed exactly when it falls in the evening or the early morning [verifies SP-1500~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			hour := rapid.IntRange(0, 23).Draw(t, "hour")
			minute := rapid.IntRange(0, 59).Draw(t, "minute")
			second := rapid.IntRange(0, 59).Draw(t, "second")
			minutes := hour*60 + minute
			want := minutes >= 21*60 || minutes < 7*60+30
			if got := nightly.Allows(at(hour, minute, second)); got != want {
				t.Fatalf("%02d:%02d:%02d allowed %v, want %v", hour, minute, second, got, want)
			}
		})
	})

	t.Run("the window describes itself in local time [verifies SP-1500~1]", func(t *testing.T) {
		if got := nightly.Describe(); got != "21:00-07:30 local time" {
			t.Errorf("Describe = %q", got)
		}
	})
}
