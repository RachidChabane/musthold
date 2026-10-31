package mn

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"pgregory.net/rapid"
)

// referenceFile is written by ../testdata/power-reference from
// experiments/loop-2/script/power.
const referenceFile = "../testdata/power-reference.json"

const (
	// referenceTolerance is relative: math.Acos near ±1 turns a last-place
	// difference from Python's libm into about 2e-9 on extreme cells.
	referenceTolerance = 1e-8
	brutalTolerance    = 1e-6
	orderTolerance     = 1e-9
	searchSteps        = 300
)

type reference struct {
	Z      float64 `json:"z"`
	Scores []struct {
		Cells  [][4]int `json:"cells"`
		Scores []struct {
			Delta float64 `json:"delta"`
			Score float64 `json:"score"`
		} `json:"scores"`
		Low  float64 `json:"low"`
		High float64 `json:"high"`
	} `json:"scores"`
}

func readReference(t *testing.T) reference {
	t.Helper()
	data, err := os.ReadFile(referenceFile)
	if err != nil {
		t.Fatal(err)
	}
	var found reference
	if err := json.Unmarshal(data, &found); err != nil {
		t.Fatal(err)
	}
	return found
}

func cellsOf(raw [][4]int) []Cell {
	cells := make([]Cell, len(raw))
	for index, cell := range raw {
		cells[index] = Cell{ControlEvents: cell[0], ControlRuns: cell[1], TreatmentEvents: cell[2], TreatmentRuns: cell[3]}
	}
	return cells
}

func near(got, want, tolerance float64) bool {
	if math.IsInf(want, 0) || math.IsInf(got, 0) {
		return got == want
	}
	return math.Abs(got-want) <= tolerance*math.Max(1, math.Abs(want))
}

func TestRestrictedMLE(t *testing.T) {
	t.Run("at delta zero the restricted rates are the pooled rate [verifies SP-2802~1]", func(t *testing.T) {
		// 3/10 control, 1/10 treatment: under p1 = p2 the maximum-likelihood rate is
		// the pooled 4/20 = 0.2 in both arms.
		r1, r2 := RestrictedMLE(0.3, 0.1, 10, 10, 0)
		if !near(r1, 0.2, 1e-12) || !near(r2, 0.2, 1e-12) {
			t.Fatalf("got %v, %v", r1, r2)
		}
		// 2/4 control, 3/12 treatment: pooled 5/16.
		r1, r2 = RestrictedMLE(0.5, 0.25, 4, 12, 0)
		if !near(r1, 5.0/16, 1e-12) || !near(r2, 5.0/16, 1e-12) {
			t.Fatalf("got %v, %v", r1, r2)
		}
	})
	t.Run("at the observed difference the restricted rates are the observed rates [verifies SP-2802~1]", func(t *testing.T) {
		r1, r2 := RestrictedMLE(0.6, 0.2, 5, 10, 0.4)
		if !near(r1, 0.6, 1e-9) || !near(r2, 0.2, 1e-9) {
			t.Fatalf("got %v, %v", r1, r2)
		}
	})
	t.Run("the closed form is the maximum of the constrained likelihood [verifies SP-2802~1]", rapid.MakeCheck(func(t *rapid.T) {
		n1 := rapid.IntRange(1, 15).Draw(t, "n1")
		n2 := rapid.IntRange(1, 15).Draw(t, "n2")
		x1 := rapid.IntRange(0, n1).Draw(t, "x1")
		x2 := rapid.IntRange(0, n2).Draw(t, "x2")
		delta := rapid.Float64Range(-0.9, 0.9).Draw(t, "delta")
		r1, r2 := RestrictedMLE(float64(x1)/float64(n1), float64(x2)/float64(n2), n1, n2, delta)
		if !near(r1-r2, delta, brutalTolerance) {
			t.Fatalf("r1 - r2 = %v, want %v", r1-r2, delta)
		}
		best := bruteRestricted(x1, n1, x2, n2, delta)
		if !near(r2, best, brutalTolerance) && logLikelihood(x1, n1, x2, n2, delta, r2) < logLikelihood(x1, n1, x2, n2, delta, best)-brutalTolerance {
			t.Fatalf("r2 = %v, the likelihood's maximum is at %v", r2, best)
		}
	}))
}

// bruteRestricted maximises the log-likelihood of the treatment rate r2, the
// control rate being r2 + delta, by golden-section search on its feasible range,
// where it is concave.
func bruteRestricted(x1, n1, x2, n2 int, delta float64) float64 {
	low, high := math.Max(0, -delta), math.Min(1, 1-delta)
	ratio := (math.Sqrt(5) - 1) / 2
	for range searchSteps {
		a := high - ratio*(high-low)
		b := low + ratio*(high-low)
		if logLikelihood(x1, n1, x2, n2, delta, a) < logLikelihood(x1, n1, x2, n2, delta, b) {
			low = a
		} else {
			high = b
		}
	}
	return (low + high) / 2
}

func logLikelihood(x1, n1, x2, n2 int, delta, r2 float64) float64 {
	r1 := r2 + delta
	return xlog(x1, r1) + xlog(n1-x1, 1-r1) + xlog(x2, r2) + xlog(n2-x2, 1-r2)
}

// xlog is k·log(rate), 0 when k is 0.
func xlog(k int, rate float64) float64 {
	if k == 0 {
		return 0
	}
	return float64(k) * math.Log(rate)
}

func TestScore(t *testing.T) {
	t.Run("a single task at delta zero is the pooled score test [verifies SP-2803~1]", func(t *testing.T) {
		// 3/10 against 1/10: pooled 0.2, weight 5, variance (0.16/10 + 0.16/10)·20/19.
		// Score = 5·0.2 / sqrt(25 · 0.032 · 20/19) = 1 / sqrt(0.8421052631578947).
		want := 1 / math.Sqrt(25*0.032*20/19)
		if got := Score([]Cell{{3, 10, 1, 10}}, 0); !near(got, want, 1e-12) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
	t.Run("a task without a run in one arm carries no weight [verifies SP-2803~1]", func(t *testing.T) {
		base := []Cell{{3, 10, 1, 10}, {2, 6, 4, 7}}
		with := append([]Cell{{1, 3, 0, 0}, {0, 0, 0, 0}}, base...)
		if Score(with, 0.1) != Score(base, 0.1) {
			t.Fatal("a cell without runs changed the score")
		}
	})
	t.Run("with no variance the score is the sign of its numerator [verifies SP-2803~1]", func(t *testing.T) {
		zero := []Cell{{0, 4, 0, 4}}
		if Score(zero, 0) != 0 || Score(nil, 0.2) != 0 {
			t.Fatal("no events at delta zero")
		}
		// All control runs escape and no treatment run does: at delta = 1 the
		// restricted rates are 1 and 0, the variance is zero and the numerator 0.
		if Score([]Cell{{4, 4, 0, 4}}, 1) != 0 {
			t.Fatal("observed difference at the bound")
		}
		// Every run escapes in both arms: at delta zero both restricted rates are 1.
		if Score([]Cell{{3, 3, 2, 2}}, 0) != 0 {
			t.Fatal("rates of one at delta zero")
		}
	})
	t.Run("the score agrees with the reference implementation [verifies SP-2803~1]", func(t *testing.T) {
		for index, c := range readReference(t).Scores {
			cells := cellsOf(c.Cells)
			for _, at := range c.Scores {
				if got := Score(cells, at.Delta); !near(got, at.Score, referenceTolerance) {
					t.Errorf("case %d, delta %v: got %v, want %v", index, at.Delta, got, at.Score)
				}
			}
		}
	})
	t.Run("the score decreases in delta when each task's arms are of equal size [verifies SP-2803~1]", rapid.MakeCheck(func(t *rapid.T) {
		cells := drawEqualCells(t)
		previous := math.Inf(1)
		for step := -99; step <= 99; step++ {
			score := Score(cells, float64(step)/100)
			if score > previous+orderTolerance*math.Max(1, math.Abs(previous)) {
				t.Fatalf("score rises from %v to %v at delta %v", previous, score, float64(step)/100)
			}
			previous = score
		}
	}))
	t.Run("with small unequal arms the score can rise, as the reference's does [verifies SP-2803~1]", func(t *testing.T) {
		// The reference implementation gives -1.9999999999999987 at delta 0 and
		// -1.999072422992767 at 0.01: the task without an escape adds variance and
		// little numerator as delta grows.
		cells := []Cell{{0, 2, 0, 4}, {0, 8, 1, 2}}
		if !near(Score(cells, 0), -1.9999999999999987, referenceTolerance) || !near(Score(cells, 0.01), -1.999072422992767, referenceTolerance) {
			t.Fatalf("got %v and %v", Score(cells, 0), Score(cells, 0.01))
		}
	})
	t.Run("swapping the arms and negating delta negates the score [verifies SP-2803~1]", rapid.MakeCheck(func(t *rapid.T) {
		cells := drawCells(t)
		delta := drawDelta(t)
		swapped := make([]Cell, len(cells))
		for index, c := range cells {
			swapped[index] = Cell{ControlEvents: c.TreatmentEvents, ControlRuns: c.TreatmentRuns, TreatmentEvents: c.ControlEvents, TreatmentRuns: c.ControlRuns}
		}
		if got, want := Score(swapped, -delta), -Score(cells, delta); !near(got, want, 1e-7) {
			t.Fatalf("swapped %v, want %v", got, want)
		}
	}))
}

// drawDelta draws a difference on a grid of hundredths. A difference closer to
// zero than the machine epsilon rounds a restricted rate of 1 + delta to 1, and
// the score of a task whose arms all escape then jumps to +inf where its true
// value tends to 0; the grid keeps the properties off that artefact, which the
// reference implementation shares.
func drawDelta(t *rapid.T) float64 {
	return float64(rapid.IntRange(-95, 95).Draw(t, "hundredths")) / 100
}

// drawEqualCells draws strata whose two arms hold the same number of runs, as
// the confirmatory design schedules them.
func drawEqualCells(t *rapid.T) []Cell {
	return rapid.SliceOfN(rapid.Custom(func(t *rapid.T) Cell {
		n := rapid.IntRange(minStratumRuns, 12).Draw(t, "n")
		return Cell{rapid.IntRange(0, n).Draw(t, "x1"), n, rapid.IntRange(0, n).Draw(t, "x2"), n}
	}), 1, 24).Draw(t, "cells")
}

// minStratumRuns is the fewest counted runs per arm of a task the analysis takes
// as a stratum (SP-2809).
const minStratumRuns = 2

// drawCells draws strata as the analysis forms them, each arm with at least
// minStratumRuns runs, and unequal arm sizes.
func drawCells(t *rapid.T) []Cell {
	return rapid.SliceOfN(rapid.Custom(func(t *rapid.T) Cell {
		n1 := rapid.IntRange(minStratumRuns, 12).Draw(t, "n1")
		n2 := rapid.IntRange(minStratumRuns, 12).Draw(t, "n2")
		return Cell{rapid.IntRange(0, n1).Draw(t, "x1"), n1, rapid.IntRange(0, n2).Draw(t, "x2"), n2}
	}), 1, 8).Draw(t, "cells")
}

func TestScoreInterval(t *testing.T) {
	t.Run("the interval agrees with the reference's score, unequal arms included [verifies SP-2804~1]", func(t *testing.T) {
		ref := readReference(t)
		if ref.Z != Z90 {
			t.Fatalf("z %v, the reference's %v", Z90, ref.Z)
		}
		for index, c := range ref.Scores {
			got := ScoreInterval(cellsOf(c.Cells), Z90)
			if !near(got.Low, c.Low, referenceTolerance) || !near(got.High, c.High, referenceTolerance) {
				t.Errorf("case %d: got [%v, %v], want [%v, %v]", index, got.Low, got.High, c.Low, c.High)
			}
		}
	})
	t.Run("the bounds are where the score crosses plus and minus z [verifies SP-2804~1]", rapid.MakeCheck(func(t *rapid.T) {
		cells := drawCells(t)
		got := ScoreInterval(cells, Z90)
		if got.Low > got.High {
			t.Fatalf("low %v above high %v", got.Low, got.High)
		}
		const step = 1e-6
		if got.Low > -1+step && Score(cells, got.Low-step) < Z90 {
			t.Fatalf("just below L = %v the score %v is under z", got.Low, Score(cells, got.Low-step))
		}
		if got.Low < 1-step && Score(cells, got.Low+step) > Z90 {
			t.Fatalf("just above L = %v the score %v is over z", got.Low, Score(cells, got.Low+step))
		}
		if got.High < 1-step && Score(cells, got.High+step) > -Z90 {
			t.Fatalf("just above U = %v the score %v is over -z", got.High, Score(cells, got.High+step))
		}
	}))
	t.Run("with equal arms L >= delta exactly when score(delta) >= z, and U < delta when score(delta) < -z [verifies SP-2804~1]", rapid.MakeCheck(func(t *rapid.T) {
		cells := drawEqualCells(t)
		got := ScoreInterval(cells, Z90)
		const step = 1e-6
		delta := drawDelta(t)
		if math.Abs(got.Low-delta) > step && (got.Low >= delta) != (Score(cells, delta) >= Z90) {
			t.Fatalf("L = %v and score(%v) = %v disagree", got.Low, delta, Score(cells, delta))
		}
		if math.Abs(got.High-delta) > step && (got.High < delta) != (Score(cells, delta) < -Z90) {
			t.Fatalf("U = %v and score(%v) = %v disagree", got.High, delta, Score(cells, delta))
		}
	}))
}
