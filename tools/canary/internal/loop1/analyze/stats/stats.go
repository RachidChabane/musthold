// Package stats holds the exact tests of the loop-1 analysis (PREREG §10): the
// stratified permutation test of the escapes, the exact enumeration of the
// killed-violation totals, the exact binomial and sign tests, and the
// Clopper–Pearson interval. Every probability a decision reads is an exact
// rational; floating point serves only the display and the interval's bisection.
// It is pure.
package stats

import (
	"math"
	"math/big"
)

// Alpha is the one-sided level of every decision (PREREG §10.2).
var Alpha = big.NewRat(1, 20)

// Significant reports whether p is at most α.
func Significant(p *big.Rat) bool { return p.Cmp(Alpha) <= 0 }

// Distribution is an exact probability distribution over the integers 0 to
// len-1: Distribution[v] is the probability of the value v.
type Distribution []*big.Rat

// Point is the distribution that puts all its mass on 0.
func Point() Distribution { return Distribution{big.NewRat(1, 1)} }

// Convolve is the distribution of the sum of two independent values.
func (d Distribution) Convolve(other Distribution) Distribution {
	sum := make(Distribution, len(d)+len(other)-1)
	for index := range sum {
		sum[index] = new(big.Rat)
	}
	for a, p := range d {
		for b, q := range other {
			sum[a+b].Add(sum[a+b], new(big.Rat).Mul(p, q))
		}
	}
	return sum
}

// At is the probability of exactly v.
func (d Distribution) At(v int) *big.Rat {
	if v < 0 || v >= len(d) {
		return new(big.Rat)
	}
	return new(big.Rat).Set(d[v])
}

// AtMost is the probability of at most v.
func (d Distribution) AtMost(v int) *big.Rat {
	total := new(big.Rat)
	for value := 0; value <= v && value < len(d); value++ {
		total.Add(total, d[value])
	}
	return total
}

// AtLeast is the probability of at least v.
func (d Distribution) AtLeast(v int) *big.Rat {
	total := new(big.Rat)
	for value := max(v, 0); value < len(d); value++ {
		total.Add(total, d[value])
	}
	return total
}

// choose is the binomial coefficient C(n, k), 0 outside 0 ≤ k ≤ n.
func choose(n, k int) *big.Int {
	if k < 0 || k > n || n < 0 {
		return new(big.Int)
	}
	return new(big.Int).Binomial(int64(n), int64(k))
}

// Stratum is one task's counted runs: each arm's run count and the runs of each
// arm in which the event happened.
type Stratum struct {
	ControlRuns, ControlEvents     int
	TreatmentRuns, TreatmentEvents int
}

func (s Stratum) events() int { return s.ControlEvents + s.TreatmentEvents }
func (s Stratum) runs() int   { return s.ControlRuns + s.TreatmentRuns }

// Hypergeometric is the distribution of the treatment arm's events of a stratum
// when the arm labels are permuted among its runs, each arm's size and the
// stratum's event total fixed.
func (s Stratum) Hypergeometric() Distribution {
	events, runs := s.events(), s.runs()
	whole := choose(runs, s.TreatmentRuns)
	distribution := make(Distribution, min(events, s.TreatmentRuns)+1)
	for treated := range distribution {
		ways := new(big.Int).Mul(choose(events, treated), choose(runs-events, s.TreatmentRuns-treated))
		distribution[treated] = new(big.Rat).SetFrac(ways, whole)
	}
	return distribution
}

// Permutation is the outcome of the exact stratified permutation test.
type Permutation struct {
	// Observed is the treatment arm's events; Floor the fewest the strata's
	// totals allow, every task's events put in its control arm as far as its
	// counts permit.
	Observed, Floor int
	// PLess is the probability of at most the observed events under the null,
	// PMore of at least that many, and PMin the smallest PLess the totals allow.
	PLess, PMore, PMin *big.Rat
}

// StratifiedPermutation runs the test of PREREG §10.2 on the strata.
func StratifiedPermutation(strata []Stratum) Permutation {
	null := Point()
	observed, floor := 0, 0
	for _, stratum := range strata {
		null = null.Convolve(stratum.Hypergeometric())
		observed += stratum.TreatmentEvents
		floor += max(0, stratum.events()-stratum.ControlRuns)
	}
	return Permutation{
		Observed: observed, Floor: floor,
		PLess: null.AtMost(observed), PMore: null.AtLeast(observed), PMin: null.AtMost(floor),
	}
}

// Valued is one task's counted runs with a non-negative value each, by arm.
type Valued struct {
	Control, Treatment []int
}

// SumDistribution enumerates every assignment of the treatment label to the
// task's runs, each arm's size fixed, and gives the distribution of the
// treatment arm's sum.
func (v Valued) SumDistribution() Distribution {
	values := append(append([]int{}, v.Control...), v.Treatment...)
	total := 0
	for _, value := range values {
		total += value
	}
	counts := make([]int64, total+1)
	assignments := int64(0)
	var walk func(start, left, sum int)
	walk = func(start, left, sum int) {
		if left == 0 {
			counts[sum]++
			assignments++
			return
		}
		for index := start; index <= len(values)-left; index++ {
			walk(index+1, left-1, sum+values[index])
		}
	}
	walk(0, len(v.Treatment), 0)
	distribution := make(Distribution, len(counts))
	for value, count := range counts {
		distribution[value] = big.NewRat(count, assignments)
	}
	return distribution
}

// SumTest is the outcome of the exact test of a treatment sum.
type SumTest struct {
	Observed int
	// PMore is the probability of at least the observed sum under the null.
	PMore *big.Rat
}

// StratifiedSum runs the test of PREREG §10.4 S2: the treatment arm's total,
// against the convolution of each task's enumerated distribution.
func StratifiedSum(tasks []Valued) SumTest {
	null := Point()
	observed := 0
	for _, task := range tasks {
		null = null.Convolve(task.SumDistribution())
		for _, value := range task.Treatment {
			observed += value
		}
	}
	return SumTest{Observed: observed, PMore: null.AtLeast(observed)}
}

// BinomialUpper is the exact probability of at least k successes in n trials of
// probability one half.
func BinomialUpper(k, n int) *big.Rat {
	ways := new(big.Int)
	for successes := max(k, 0); successes <= n; successes++ {
		ways.Add(ways, choose(n, successes))
	}
	return new(big.Rat).SetFrac(ways, new(big.Int).Lsh(big.NewInt(1), uint(n)))
}

// Sign is the outcome of the exact one-sided sign test.
type Sign struct {
	// Favourable counts the positive differences, Informative the non-zero ones.
	Favourable, Informative int
	P                       *big.Rat
}

// SignTest runs the exact one-sided sign test of positive differences; a zero
// difference is dropped, and with none left p is 1.
func SignTest(differences []*big.Rat) Sign {
	test := Sign{}
	for _, difference := range differences {
		switch difference.Sign() {
		case 1:
			test.Favourable++
			test.Informative++
		case -1:
			test.Informative++
		}
	}
	test.P = BinomialUpper(test.Favourable, test.Informative)
	return test
}

// Interval is an exact interval of a proportion.
type Interval struct{ Low, High float64 }

// Confidence is the two-sided level of every interval.
const Confidence = 0.95

const (
	bisectionSteps = 200
	tail           = (1 - Confidence) / 2
)

// ClopperPearson is the exact interval of k successes in n trials, found by
// inverting the binomial distribution function by bisection. With no trial it
// is [0, 1].
func ClopperPearson(k, n int) Interval {
	if n <= 0 {
		return Interval{Low: 0, High: 1}
	}
	interval := Interval{Low: 0, High: 1}
	if k > 0 {
		// The lower bound is the rate at which k or more successes have probability
		// tail: 1 - F(k-1) rises with the rate.
		interval.Low = bisect(func(rate float64) bool { return 1-binomialCDF(k-1, n, rate) < tail })
	}
	if k < n {
		// The upper bound is the rate at which k or fewer successes have
		// probability tail: F(k) falls as the rate rises.
		interval.High = bisect(func(rate float64) bool { return binomialCDF(k, n, rate) > tail })
	}
	return interval
}

// bisect finds the point of [0, 1] where below turns from true to false.
func bisect(below func(rate float64) bool) float64 {
	low, high := 0.0, 1.0
	for range bisectionSteps {
		middle := (low + high) / 2
		if below(middle) {
			low = middle
		} else {
			high = middle
		}
	}
	return (low + high) / 2
}

// binomialCDF is the probability of at most k successes in n trials of the rate.
func binomialCDF(k, n int, rate float64) float64 {
	total := 0.0
	for successes := 0; successes <= k; successes++ {
		coefficient, _ := new(big.Float).SetInt(choose(n, successes)).Float64()
		total += coefficient * math.Pow(rate, float64(successes)) * math.Pow(1-rate, float64(n-successes))
	}
	return total
}
