package decision

import (
	"encoding/json"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/stats"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/mn"
)

// referenceFile is written by ../testdata/power-reference from
// experiments/loop-2/script/power.
const (
	referenceFile = "../testdata/power-reference.json"
	tailTolerance = 1e-12
)

var (
	significant = big.NewRat(1, 100)
	notSig      = big.NewRat(1, 2)
)

func TestGuards(t *testing.T) {
	t.Run("G holds up to 3 of 20 refused oracle-clean changes and when there is none [verifies SP-2805~1]", func(t *testing.T) {
		cases := []struct {
			refused, clean int
			holds          bool
		}{
			{3, 20, true}, {4, 20, false}, {0, 0, true}, {1, 6, false}, {1, 7, true}, {0, 1, true}, {28, 192, true}, {29, 192, false},
		}
		for _, c := range cases {
			got := EvaluateGuards(Counts{TreatmentClean: c.clean, TreatmentRefusedClean: c.refused})
			if got.G.Holds != c.holds {
				t.Errorf("%d/%d refused: holds %v, want %v", c.refused, c.clean, got.G.Holds, c.holds)
			}
		}
	})
	t.Run("Y holds while the treatment's clean accepted share is at most 1/10 below the control's, each over its own runs [verifies SP-2806~1]", func(t *testing.T) {
		cases := []struct {
			controlK, controlN, treatK, treatN int
			holds                              bool
		}{
			// 150/192 - 131/192 = 19/192 <= 1/10; 150/192 - 130/192 = 20/192 > 1/10.
			{150, 192, 131, 192, true}, {150, 192, 130, 192, false},
			// Unequal arms: 9/10 - 8/10 = 1/10 holds; 9/10 - 23/29 = 0.1069 fails.
			{9, 10, 8, 10, true}, {9, 10, 23, 29, false},
			// A treatment above the control always holds.
			{1, 10, 10, 10, true},
			// An arm without a run has a share of 0.
			{0, 0, 0, 0, true}, {5, 10, 0, 0, false},
		}
		for _, c := range cases {
			got := EvaluateGuards(Counts{ControlAcceptedClean: c.controlK, ControlRuns: c.controlN, TreatmentAcceptedClean: c.treatK, TreatmentRuns: c.treatN})
			if got.Y.Holds != c.holds {
				t.Errorf("%d/%d against %d/%d: holds %v, want %v", c.controlK, c.controlN, c.treatK, c.treatN, got.Y.Holds, c.holds)
			}
		}
	})
}

// referenceDecide is the decision rule written as the reference implementation
// writes it, nested.
func referenceDecide(less, more bool, score float64, guards bool) Outcome {
	lowerClears, upperBelow := score >= Z, score < -Z
	if more {
		return Harm
	}
	if less {
		if !guards {
			return RejectionOnly
		}
		if lowerClears {
			return UsefulGain
		}
		if upperBelow {
			return GainBelowUseful
		}
		return Gain
	}
	if upperBelow {
		return NoUsefulGain
	}
	return Inconclusive
}

func TestDecide(t *testing.T) {
	t.Run("hand-computed rows in order [verifies SP-2807~1]", func(t *testing.T) {
		cases := []struct {
			name       string
			less, more *big.Rat
			score      float64
			guards     bool
			want       Outcome
		}{
			{"more escapes in treatment", notSig, big.NewRat(1, 20), -3, true, Harm},
			{"harm before rejection-only even with both tails low", significant, significant, 3, false, Harm},
			{"fewer escapes with G or Y failing", significant, notSig, 3, false, RejectionOnly},
			{"fewer escapes, L at least delta", significant, notSig, Z, true, UsefulGain},
			{"fewer escapes, U below delta", significant, notSig, -Z - 1e-9, true, GainBelowUseful},
			{"fewer escapes, U at delta", significant, notSig, -Z, true, Gain},
			{"fewer escapes at exactly alpha", big.NewRat(1, 20), notSig, 0, true, Gain},
			{"no direction, U below delta", notSig, notSig, -2, true, NoUsefulGain},
			{"no direction, guards ignored", notSig, notSig, -2, false, NoUsefulGain},
			{"no direction, interval spans delta", notSig, notSig, 0, true, Inconclusive},
			{"p just above alpha", big.NewRat(51, 1000), big.NewRat(51, 1000), 0, true, Inconclusive},
		}
		for _, c := range cases {
			got := Decide(Evidence{PLess: c.less, PMore: c.more, ScoreAtMargin: c.score, GuardsHold: c.guards})
			if got.Outcome != c.want {
				t.Errorf("%s: %s, want %s", c.name, got.Outcome, c.want)
			}
		}
	})
	t.Run("exhaustive on the grid of tails, score regions and guards [verifies SP-2807~1]", func(t *testing.T) {
		tails := []*big.Rat{significant, big.NewRat(1, 20), big.NewRat(51, 1000), notSig}
		scores := []float64{math.Inf(-1), -Z - 1, -Z - 1e-12, -Z, 0, Z - 1e-12, Z, Z + 1, math.Inf(1)}
		seen := map[Outcome]bool{}
		for _, less := range tails {
			for _, more := range tails {
				for _, score := range scores {
					for _, g := range []bool{true, false} {
						for _, y := range []bool{true, false} {
							got := Decide(Evidence{PLess: less, PMore: more, ScoreAtMargin: score, GuardsHold: g && y})
							want := referenceDecide(stats.Significant(less), stats.Significant(more), score, g && y)
							if got.Outcome != want {
								t.Errorf("less %s more %s score %v G %v Y %v: %s, want %s", less.RatString(), more.RatString(), score, g, y, got.Outcome, want)
							}
							seen[got.Outcome] = true
						}
					}
				}
			}
		}
		if len(seen) != len(Table()) {
			t.Fatalf("the grid reached %d outcomes of %d", len(seen), len(Table()))
		}
	})
	t.Run("the outcome is the first row that holds on any evidence [verifies SP-2807~1]", rapid.MakeCheck(func(t *rapid.T) {
		e := Evidence{
			PLess:         big.NewRat(int64(rapid.IntRange(0, 1000).Draw(t, "less")), 1000),
			PMore:         big.NewRat(int64(rapid.IntRange(0, 1000).Draw(t, "more")), 1000),
			ScoreAtMargin: rapid.Float64Range(-5, 5).Draw(t, "score"),
			GuardsHold:    rapid.Bool().Draw(t, "guards"),
		}
		got := Decide(e)
		if want := referenceDecide(stats.Significant(e.PLess), stats.Significant(e.PMore), e.ScoreAtMargin, e.GuardsHold); got.Outcome != want {
			t.Fatalf("%s, want %s", got.Outcome, want)
		}
		for _, row := range Table()[:got.Number-1] {
			if row.holds(e) {
				t.Fatalf("row %d holds before row %d", row.Number, got.Number)
			}
		}
	}))
	t.Run("the decision agrees with the reference implementation on its own inputs [verifies SP-2807~1]", func(t *testing.T) {
		data, err := os.ReadFile(referenceFile)
		if err != nil {
			t.Fatal(err)
		}
		var ref struct {
			Delta     float64 `json:"delta"`
			Decisions []struct {
				Cells         [][4]int `json:"cells"`
				GuardsHold    bool     `json:"guardsHold"`
				PLess         float64  `json:"pLess"`
				PMore         float64  `json:"pMore"`
				ScoreAtMargin float64  `json:"scoreAtMargin"`
				Outcome       Outcome  `json:"outcome"`
			} `json:"decisions"`
		}
		if err := json.Unmarshal(data, &ref); err != nil {
			t.Fatal(err)
		}
		if ref.Delta != Margin {
			t.Fatalf("margin %v, the reference's %v", Margin, ref.Delta)
		}
		for index, c := range ref.Decisions {
			var strata []stats.Stratum
			var cells []mn.Cell
			for _, cell := range c.Cells {
				strata = append(strata, stats.Stratum{ControlEvents: cell[0], ControlRuns: cell[1], TreatmentEvents: cell[2], TreatmentRuns: cell[3]})
				cells = append(cells, mn.Cell{ControlEvents: cell[0], ControlRuns: cell[1], TreatmentEvents: cell[2], TreatmentRuns: cell[3]})
			}
			test := stats.StratifiedPermutation(strata)
			less, _ := test.PLess.Float64()
			more, _ := test.PMore.Float64()
			score := mn.Score(cells, Margin)
			if math.Abs(less-c.PLess) > tailTolerance || math.Abs(more-c.PMore) > tailTolerance {
				t.Errorf("case %d: tails %v, %v, want %v, %v", index, less, more, c.PLess, c.PMore)
			}
			if math.Abs(score-c.ScoreAtMargin) > 1e-8*math.Max(1, math.Abs(c.ScoreAtMargin)) {
				t.Errorf("case %d: score %v, want %v", index, score, c.ScoreAtMargin)
			}
			got := Decide(Evidence{PLess: test.PLess, PMore: test.PMore, ScoreAtMargin: score, GuardsHold: c.GuardsHold})
			if got.Outcome != c.Outcome {
				t.Errorf("case %d: %s, want %s", index, got.Outcome, c.Outcome)
			}
		}
	})
}

func TestReading(t *testing.T) {
	t.Run("rows 3 and 5 show a gain, rows 1, 2, 4 and 6 no useful gain, row 7 decides nothing [verifies SP-2808~1]", func(t *testing.T) {
		want := map[Outcome]Verdict{
			Harm: VerdictNoUsefulGain, RejectionOnly: VerdictNoUsefulGain, UsefulGain: VerdictGain,
			GainBelowUseful: VerdictNoUsefulGain, Gain: VerdictGain, NoUsefulGain: VerdictNoUsefulGain, Inconclusive: VerdictUndecided,
		}
		for number, row := range Table() {
			if row.Number != number+1 || row.Verdict != want[row.Outcome] || RowOf(row.Outcome).Number != row.Number {
				t.Errorf("row %+v", row)
			}
			if GainShown(row.Outcome) != (row.Outcome == UsefulGain || row.Outcome == Gain) {
				t.Errorf("%s: gain shown %v", row.Outcome, GainShown(row.Outcome))
			}
		}
	})
	t.Run("the readings are ADR 0017's [verifies SP-2808~1]", func(t *testing.T) {
		data, err := os.ReadFile("../../../../../docs/adr/0017-the-loop-2-decision.md")
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range Table()[:len(Table())-1] {
			if !containsRow(string(data), row) {
				t.Errorf("ADR 0017 has no row %d %s with reading %q", row.Number, row.Outcome, row.Reading)
			}
		}
	})
}

func containsRow(adr string, row Row) bool {
	needle := "| " + strconv.Itoa(row.Number) + " | " + string(row.Outcome) + " | "
	at := strings.Index(adr, needle)
	if at < 0 {
		return false
	}
	line, _, _ := strings.Cut(adr[at:], "\n")
	return strings.Contains(line, "| "+row.Reading+" |")
}
