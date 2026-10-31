// Package mn holds the stratified Miettinen–Nurminen score of the risk
// difference, control minus treatment, with tasks as strata and weights
// n1·n2/(n1+n2) (ADR 0017, "Outcome"), and the score interval found by
// root-finding on that score. It agrees with experiments/loop-2/script/power,
// the reference implementation. It is pure.
package mn

import "math"

// Z90 is the standard normal quantile of a two-sided 0.90 interval, the level of
// the magnitude in ADR 0017.
const Z90 = 1.6448536269514722

// Bounds of the risk difference, and how many halvings the root-finding takes.
const (
	lowestDifference  = -1.0
	highestDifference = 1.0
	bisectionSteps    = 200
)

// Cell is one task's counted runs: each arm's events and runs. The control arm
// is arm 1 and the treatment arm arm 2, so the difference is control minus
// treatment.
type Cell struct {
	ControlEvents, ControlRuns     int
	TreatmentEvents, TreatmentRuns int
}

// informative reports whether both arms hold a run; a cell without one carries
// no weight and is left out.
func (c Cell) informative() bool { return c.ControlRuns > 0 && c.TreatmentRuns > 0 }

func (c Cell) rates() (p1, p2 float64) {
	return float64(c.ControlEvents) / float64(c.ControlRuns), float64(c.TreatmentEvents) / float64(c.TreatmentRuns)
}

// weight is n1·n2/(n1+n2).
func (c Cell) weight() float64 {
	n1, n2 := float64(c.ControlRuns), float64(c.TreatmentRuns)
	return n1 * n2 / (n1 + n2)
}

// RestrictedMLE is the maximum-likelihood pair of rates of a cell under the
// constraint that the control rate minus the treatment rate is delta: the
// closed-form root of the Miettinen–Nurminen cubic, clamped to [0, 1].
func RestrictedMLE(p1, p2 float64, n1, n2 int, delta float64) (r1, r2 float64) {
	theta := float64(n2) / float64(n1)
	a := 1 + theta
	b := -(1 + theta + p1 + theta*p2 + delta*(theta+2))
	c := delta*delta + delta*(2*p1+theta+1) + p1 + theta*p2
	d := -p1 * delta * (1 + delta)
	v := math.Pow(b, 3)/math.Pow(3*a, 3) - b*c/(6*a*a) + d/(2*a)
	s := b*b/math.Pow(3*a, 2) - c/(3*a)
	u := math.Sqrt(math.Max(s, 0))
	if v != 0 {
		u = math.Copysign(u, v)
	}
	if u == 0 {
		r1 = -b / (3 * a)
	} else {
		w := (math.Pi + math.Acos(math.Max(-1, math.Min(1, v/math.Pow(u, 3))))) / 3
		r1 = 2*u*math.Cos(w) - b/(3*a)
	}
	r1 = clamp(r1)
	return r1, clamp(r1 - delta)
}

func clamp(rate float64) float64 { return math.Min(1, math.Max(0, rate)) }

// Score is the stratified Miettinen–Nurminen score at delta. It decreases in
// delta when each task's arms are of equal size; with small unequal arms it can
// rise locally. With no variance it is +∞, −∞ or 0 by the sign of its numerator.
func Score(cells []Cell, delta float64) float64 {
	numerator, denominator := 0.0, 0.0
	for _, cell := range cells {
		if !cell.informative() {
			continue
		}
		p1, p2 := cell.rates()
		r1, r2 := RestrictedMLE(p1, p2, cell.ControlRuns, cell.TreatmentRuns, delta)
		n1, n2 := float64(cell.ControlRuns), float64(cell.TreatmentRuns)
		variance := (r1*(1-r1)/n1 + r2*(1-r2)/n2) * (n1 + n2) / (n1 + n2 - 1)
		weight := cell.weight()
		numerator += weight * (p1 - p2 - delta)
		denominator += weight * weight * variance
	}
	if denominator <= 0 {
		switch {
		case numerator > 0:
			return math.Inf(1)
		case numerator < 0:
			return math.Inf(-1)
		default:
			return 0
		}
	}
	return numerator / math.Sqrt(denominator)
}

// Interval is the score interval [Low, High] of the risk difference.
type Interval struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}

// ScoreInterval is the interval of the cells at the quantile z: Low is where the
// score falls to z and High where it falls to −z, each found by bisection on
// [−1, 1]. Where the score rises locally, bisection finds one of its crossings.
func ScoreInterval(cells []Cell, z float64) Interval {
	return Interval{Low: root(cells, z), High: root(cells, -z)}
}

// root finds the difference at which the decreasing score crosses target.
func root(cells []Cell, target float64) float64 {
	low, high := lowestDifference, highestDifference
	for range bisectionSteps {
		middle := (low + high) / 2
		if Score(cells, middle) >= target {
			low = middle
		} else {
			high = middle
		}
	}
	return (low + high) / 2
}
