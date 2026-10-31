// Package decision holds the outcome table of the loop-2 study (ADR 0017,
// "Outcome"): guards G and Y, the seven rows taken in order, and the reading of
// each outcome. It is pure.
package decision

import (
	"math/big"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/stats"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/mn"
)

// Margin is δ, the smallest risk difference called useful.
const Margin = 0.15

// Z is the quantile of the 0.90 score interval the magnitude reads.
const Z = mn.Z90

// GuardGBound is the largest share of its oracle-clean changes the treatment
// gate may refuse; GuardYBound the largest shortfall of the treatment's share
// of runs ending in an accepted oracle-clean change below the control's.
var (
	GuardGBound = big.NewRat(3, 20)
	GuardYBound = big.NewRat(1, 10)
)

// Outcome is a row of the table.
type Outcome string

// The outcomes, in the order of the table.
const (
	Harm            Outcome = "HARM"
	RejectionOnly   Outcome = "REJECTION-ONLY"
	UsefulGain      Outcome = "USEFUL GAIN"
	GainBelowUseful Outcome = "GAIN BELOW USEFUL"
	Gain            Outcome = "GAIN"
	NoUsefulGain    Outcome = "NO USEFUL GAIN"
	Inconclusive    Outcome = "INCONCLUSIVE"
)

// Verdict is what an outcome demonstrates.
type Verdict string

// The verdicts of ADR 0017's "Reading".
const (
	VerdictGain         Verdict = "a gain is shown"
	VerdictNoUsefulGain Verdict = "the product, as tested, brings no useful gain"
	VerdictUndecided    Verdict = "the study does not decide"
)

// Counts are the counted runs the guards read, per arm.
type Counts struct {
	// TreatmentClean is the treatment runs whose final change is oracle-clean,
	// and TreatmentRefusedClean those the gate refused.
	TreatmentClean, TreatmentRefusedClean int
	// ControlRuns and TreatmentRuns are each arm's counted runs; the
	// AcceptedClean counts are the runs ending in an accepted oracle-clean
	// change.
	ControlRuns, ControlAcceptedClean     int
	TreatmentRuns, TreatmentAcceptedClean int
}

// Guards are guards G and Y with what they read.
type Guards struct {
	G GuardG `json:"g"`
	Y GuardY `json:"y"`
}

// Hold reports whether both guards hold.
func (g Guards) Hold() bool { return g.G.Holds && g.Y.Holds }

// GuardG is the treatment's refused oracle-clean changes over its oracle-clean
// changes, against GuardGBound.
type GuardG struct {
	Refused int  `json:"refused"`
	Clean   int  `json:"clean"`
	Holds   bool `json:"holds"`
}

// GuardY is each arm's runs ending in an accepted oracle-clean change over its
// counted runs; it holds when the control's share exceeds the treatment's by at
// most GuardYBound.
type GuardY struct {
	ControlAcceptedClean   int  `json:"controlAcceptedClean"`
	ControlRuns            int  `json:"controlRuns"`
	TreatmentAcceptedClean int  `json:"treatmentAcceptedClean"`
	TreatmentRuns          int  `json:"treatmentRuns"`
	Holds                  bool `json:"holds"`
}

// EvaluateGuards evaluates G and Y on the counts. G holds when the treatment
// has no oracle-clean change; a share over an arm without a run is 0.
func EvaluateGuards(c Counts) Guards {
	g := GuardG{Refused: c.TreatmentRefusedClean, Clean: c.TreatmentClean}
	g.Holds = share(g.Refused, g.Clean).Cmp(GuardGBound) <= 0
	y := GuardY{
		ControlAcceptedClean: c.ControlAcceptedClean, ControlRuns: c.ControlRuns,
		TreatmentAcceptedClean: c.TreatmentAcceptedClean, TreatmentRuns: c.TreatmentRuns,
	}
	shortfall := new(big.Rat).Sub(share(y.ControlAcceptedClean, y.ControlRuns), share(y.TreatmentAcceptedClean, y.TreatmentRuns))
	y.Holds = shortfall.Cmp(GuardYBound) <= 0
	return Guards{G: g, Y: y}
}

func share(k, n int) *big.Rat {
	if n == 0 {
		return new(big.Rat)
	}
	return big.NewRat(int64(k), int64(n))
}

// Evidence is what the table reads: the tails of the exact stratified
// permutation test, the score at the margin and whether the guards hold.
type Evidence struct {
	PLess, PMore  *big.Rat
	ScoreAtMargin float64
	GuardsHold    bool
}

// lowerClears is L ≥ δ, read as score(δ) ≥ z.
func (e Evidence) lowerClears() bool { return e.ScoreAtMargin >= Z }

// upperBelow is U < δ, read as score(δ) < −z.
func (e Evidence) upperBelow() bool { return e.ScoreAtMargin < -Z }

func (e Evidence) less() bool { return stats.Significant(e.PLess) }
func (e Evidence) more() bool { return stats.Significant(e.PMore) }

// Row is one row of the table: its number, its outcome, its rule, its reading
// and what it demonstrates.
type Row struct {
	Number  int
	Outcome Outcome
	Reading string
	Verdict Verdict
	holds   func(Evidence) bool
}

// Table is ADR 0017's table, in order.
func Table() []Row {
	return []Row{
		{1, Harm, "the product makes things worse", VerdictNoUsefulGain, Evidence.more},
		{2, RejectionOnly, "fewer escapes, bought by refusing correct work", VerdictNoUsefulGain,
			func(e Evidence) bool { return e.less() && !e.GuardsHold }},
		{3, UsefulGain, "gain shown, and it is useful", VerdictGain,
			func(e Evidence) bool { return e.less() && e.lowerClears() }},
		{4, GainBelowUseful, "a real gain, too small for its cost", VerdictNoUsefulGain,
			func(e Evidence) bool { return e.less() && e.upperBelow() }},
		{5, Gain, "gain shown; whether it reaches δ stays open", VerdictGain, Evidence.less},
		{6, NoUsefulGain, "any gain is below what would be useful", VerdictNoUsefulGain, Evidence.upperBelow},
		{7, Inconclusive, "the study cannot decide", VerdictUndecided, func(Evidence) bool { return true }},
	}
}

// Decide returns the first row of the table that holds.
func Decide(e Evidence) Row {
	table := Table()
	for _, row := range table {
		if row.holds(e) {
			return row
		}
	}
	return table[len(table)-1]
}

// RowOf returns the row of an outcome.
func RowOf(outcome Outcome) Row {
	table := Table()
	for _, row := range table {
		if row.Outcome == outcome {
			return row
		}
	}
	return table[len(table)-1]
}

// GainShown reports whether the outcome demonstrates a gain: rows 3 and 5.
func GainShown(outcome Outcome) bool { return RowOf(outcome).Verdict == VerdictGain }
