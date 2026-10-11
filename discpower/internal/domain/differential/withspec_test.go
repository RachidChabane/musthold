package differential_test

import (
	"fmt"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
)

const (
	whenParam = "when"
	rateParam = "rate"
	modeParam = "mode"

	maxHour   = 23
	maxMinute = 59
)

func datetimeParam() differential.Param {
	return differential.Param{Name: whenParam, Hint: differential.HintDatetime}
}

func drawnDatetimes(text string) []string {
	pool := differential.NewPool(nil, nil).WithSpec([]string{text})
	return valuesOf(pool.Draw(signatureOf(datetimeParam()), seed, manyDraws), whenParam)
}

func TestWithSpec(t *testing.T) {
	for _, c := range []struct {
		name string
		text string
		want []string
	}{
		{"a time of day is drawn on the anchor day and the next one", "stored at 22:00", []string{"2024-01-01T22:00:00", "2024-01-02T22:00:00"}},
		{"a time of day with seconds is drawn", "stored at 10:00:30", []string{"2024-01-01T10:00:30", "2024-01-02T10:00:30"}},
		{"a duration in minutes is counted from the anchor midnight", "within 15 minutes", []string{"2024-01-01T00:15:00"}},
		{"a hyphenated duration is counted from the anchor midnight", "a 24-hour period", []string{"2024-01-02T00:00:00"}},
		{"a duration in days is counted from the anchor midnight", "after 2 days", []string{"2024-01-03T00:00:00"}},
		{"the neighbours a microsecond away are drawn", "stored at 22:00", []string{"2024-01-01T22:00:00.000001", "2024-01-01T21:59:59.999999"}},
		{"the neighbours a second away are drawn", "stored at 22:00", []string{"2024-01-01T22:00:01", "2024-01-01T21:59:59"}},
		{"the neighbours a minute away are drawn", "stored at 22:00", []string{"2024-01-01T22:01:00", "2024-01-01T21:59:00"}},
	} {
		t.Run(c.name+verifiesSP170, func(t *testing.T) {
			values := drawnDatetimes(c.text)
			for _, want := range c.want {
				if !slices.Contains(values, want) {
					t.Errorf("%s is missing from %v", want, values)
				}
			}
		})
	}
	t.Run("a spec naming no instant adds only the basic datetimes"+verifiesSP170, func(t *testing.T) {
		if got, want := len(drawnDatetimes("the amount is rounded")), len(drawnDatetimes("")); got != want {
			t.Errorf("drew %d datetimes, want %d", got, want)
		}
	})
	t.Run("a number of the spec is drawn with finer and half-step neighbours"+verifiesSP170, func(t *testing.T) {
		pool := differential.NewPool(nil, nil).WithSpec([]string{"the rate is 2.50 per unit"})
		values := valuesOf(pool.Draw(signatureOf(differential.Param{Name: rateParam, Hint: differential.HintDecimal}), seed, manyDraws), rateParam)
		for _, want := range []string{"2.50", "2.495", "2.505", "2.49", "2.51", "3.50", "1.50"} {
			if !slices.Contains(values, want) {
				t.Errorf("%s is missing from %v", want, values)
			}
		}
	})
	t.Run("a quoted text of the spec is drawn and a long one is not"+verifiesSP170, func(t *testing.T) {
		long := "a quoted text longer than twenty four characters"
		pool := differential.NewPool(nil, nil).WithSpec([]string{`the mode "express" applies, not "` + long + `"`})
		values := valuesOf(pool.Draw(signatureOf(differential.Param{Name: modeParam, Hint: differential.HintString}), seed, manyDraws), modeParam)
		if !slices.Contains(values, "express") || slices.Contains(values, long) {
			t.Errorf("got %v", values)
		}
	})
	t.Run("seeding returns a new pool and leaves the original unchanged"+verifiesSP170, func(t *testing.T) {
		pool := differential.NewPool(nil, nil)
		before := valuesOf(pool.Draw(signatureOf(datetimeParam()), seed, manyDraws), whenParam)
		pool.WithSpec([]string{"stored at 22:00"})
		after := valuesOf(pool.Draw(signatureOf(datetimeParam()), seed, manyDraws), whenParam)
		if !slices.Equal(before, after) {
			t.Errorf("the pool changed: %v then %v", before, after)
		}
	})
	t.Run("seeding keeps the values of the code"+verifiesSP170, func(t *testing.T) {
		pool := differential.NewPool([]string{"limit = 77"}, nil).WithSpec([]string{"stored at 22:00"})
		values := valuesOf(pool.Draw(signatureOf(integerParam("n")), seed, manyDraws), "n")
		for _, want := range []string{"76", "77", "78", "22"} {
			if !slices.Contains(values, want) {
				t.Errorf("%s is missing from %v", want, values)
			}
		}
	})
	t.Run("every time of day a spec names is drawn on both days"+verifiesSP170, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			hour := rapid.IntRange(0, maxHour).Draw(t, "hour")
			minute := rapid.IntRange(0, maxMinute).Draw(t, "minute")
			values := drawnDatetimes(fmt.Sprintf("closes at %02d:%02d", hour, minute))
			for _, day := range []string{"2024-01-01", "2024-01-02"} {
				want := fmt.Sprintf("%sT%02d:%02d:00", day, hour, minute)
				if !slices.Contains(values, want) {
					t.Fatalf("%s is missing from %v", want, values)
				}
			}
		})
	})
}
