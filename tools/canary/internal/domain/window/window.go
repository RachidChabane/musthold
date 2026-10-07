// Package window is the nightly window in which a live canary may run: the
// session spends model quota the user needs by day.
package window

import (
	"fmt"
	"time"
)

// Local-time bounds of the window. It wraps midnight: it opens in the evening and
// closes in the morning.
const (
	OpensHour    = 21
	OpensMinute  = 0
	ClosesHour   = 7
	ClosesMinute = 30
)

// Window is a daily interval in local time that wraps midnight.
type Window struct {
	opens, closes time.Duration
}

func clock(hour, minute int) time.Duration {
	return time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute
}

// Nightly is the window of 21:00 to 07:30.
func Nightly() Window {
	return Window{opens: clock(OpensHour, OpensMinute), closes: clock(ClosesHour, ClosesMinute)}
}

// Allows reports whether the instant, read in its own location, is inside the
// window: from the opening time inclusive to the closing time exclusive.
func (w Window) Allows(instant time.Time) bool {
	sinceMidnight := clock(instant.Hour(), instant.Minute()) +
		time.Duration(instant.Second())*time.Second + time.Duration(instant.Nanosecond())
	return sinceMidnight >= w.opens || sinceMidnight < w.closes
}

// Describe is the window as a person reads it.
func (w Window) Describe() string {
	return fmt.Sprintf("%02d:%02d-%02d:%02d local time", OpensHour, OpensMinute, ClosesHour, ClosesMinute)
}
