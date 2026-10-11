package scan

import (
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
)

// Depth is how much of a suite a scan judges: a quick scan gives a first
// number within a budget of about a minute and a half; a full scan judges the
// default sample against the coverage of every green test, with no budget.
type Depth int

// Depths of a scan.
const (
	// DepthFull samples DefaultSampleSize tests, draws DefaultMutants, records
	// the coverage of every green test and runs every mutant.
	DepthFull Depth = iota
	// DepthQuick samples QuickSampleSize tests, draws CandidatesPerTest mutants
	// for each, records the coverage of the sampled tests only, and starts no
	// mutant once QuickBudget has passed.
	DepthQuick
)

// Settings of a quick scan.
const (
	// QuickSampleSize is how many green tests a quick scan samples.
	QuickSampleSize = 12
	// QuickMutants is how many operator mutants a quick scan draws: the
	// allocation's share of each sampled test.
	QuickMutants = QuickSampleSize * scanscope.CandidatesPerTest
	// QuickBudget is how long after it began a quick scan starts no mutant.
	QuickBudget = 90 * time.Second
)

// Preset is what a depth sets when the request does not.
type Preset struct {
	SampleSize int
	Mutants    int
	// Budget is zero for no budget.
	Budget   time.Duration
	Coverage CoverageScope
}

// PresetOf is the preset of a depth.
func PresetOf(depth Depth) Preset {
	if depth == DepthQuick {
		return Preset{SampleSize: QuickSampleSize, Mutants: QuickMutants, Budget: QuickBudget, Coverage: CoverSampled}
	}
	return Preset{SampleSize: scanscope.DefaultSampleSize, Mutants: scanscope.DefaultMutants, Coverage: CoverGreen}
}
