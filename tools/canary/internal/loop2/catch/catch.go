// Package catch holds the catches of the loop-2 study and their attribution
// (ADR 0017, "Secondary analyses"): which treatment runs the gate caught, the
// finding each catch is owed to, the two-thirds rule of the spec-witness check,
// and the runs of the mutation-only recount. It is pure.
package catch

import (
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/decision"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
)

// Kind is how the gate caught a run.
type Kind string

// The kinds of catch.
const (
	// KindCorrected: the tree at the first stop is oracle-defective and the
	// final change is oracle-clean and accepted.
	KindCorrected Kind = "corrected"
	// KindRefused: the final change is oracle-defective and refused by a
	// finding that names a violated criterion.
	KindRefused Kind = "refused"
)

// Attribution is the class of the finding a catch is owed to.
type Attribution string

// The attributions, in the order of precedence.
const (
	SpecWitness     Attribution = "spec_witness_failed"
	SpecExample     Attribution = "example_failed"
	SurvivingMutant Attribution = "surviving_valid_mutant"
	Alignment       Attribution = "alignment"
	Other           Attribution = "other"
	None            Attribution = "none"
)

// Attributions lists the attributions in the order of precedence.
func Attributions() []Attribution {
	return []Attribution{SpecWitness, SpecExample, SurvivingMutant, Alignment, Other, None}
}

// AlignmentModule is the module whose findings are alignment findings.
const AlignmentModule = "musthold-alignment"

// codeAttributions maps the finding codes that name their class.
var codeAttributions = map[string]Attribution{
	string(SpecWitness):     SpecWitness,
	string(SpecExample):     SpecExample,
	string(SurvivingMutant): SurvivingMutant,
}

func classOf(finding facts.Finding) Attribution {
	if class, found := codeAttributions[finding.Code]; found {
		return class
	}
	if finding.Module == AlignmentModule {
		return Alignment
	}
	return Other
}

// Attribute is the first class the findings hold, in the order of precedence,
// or None when they hold no finding.
func Attribute(findings []facts.Finding) Attribution {
	best := None
	for _, finding := range findings {
		class := classOf(finding)
		if slices.Index(Attributions(), class) < slices.Index(Attributions(), best) {
			best = class
		}
	}
	return best
}

// Catch is one caught treatment run with the findings it is attributed by.
type Catch struct {
	Run         string          `json:"run"`
	Task        string          `json:"task"`
	Kind        Kind            `json:"kind"`
	Attribution Attribution     `json:"attribution"`
	Findings    []facts.Finding `json:"findings"`
}

// Of reports whether a run is a catch, and the catch it is.
func Of(run record.Run) (Catch, bool) {
	if run.Arm != analyze.ArmTreatment {
		return Catch{}, false
	}
	switch {
	case run.FirstStop != nil && run.FirstStop.Defective && !run.Defective && run.Accepted:
		findings := []facts.Finding{}
		if block, found := run.LastBlock(facts.KindFinding); found {
			findings = block.Findings
		}
		return newCatch(run, KindCorrected, findings), true
	case run.Defective && !run.Accepted && run.Refusal.Cause == record.CauseFinding:
		naming := namingViolated(run.Refusal.Findings, run.Violated)
		return newCatch(run, KindRefused, naming), len(naming) > 0
	}
	return Catch{}, false
}

func newCatch(run record.Run, kind Kind, findings []facts.Finding) Catch {
	return Catch{Run: run.Run, Task: run.Task, Kind: kind, Attribution: Attribute(findings), Findings: findings}
}

// namingViolated keeps the findings that name a violated criterion.
func namingViolated(findings []facts.Finding, violated []string) []facts.Finding {
	kept := []facts.Finding{}
	for _, finding := range findings {
		if slices.Contains(violated, finding.Criterion) {
			kept = append(kept, finding)
		}
	}
	return kept
}

// Find lists the catches among the runs, in their order.
func Find(runs []record.Run) []Catch {
	catches := []Catch{}
	for _, run := range runs {
		if caught, ok := Of(run); ok {
			catches = append(catches, caught)
		}
	}
	return catches
}

// Summary is the catches with their count per attribution and the two-thirds
// rule of the spec-witness check.
type Summary struct {
	Catches       []Catch             `json:"catches"`
	ByAttribution map[Attribution]int `json:"byAttribution"`
	// SpecWitness is the catches attributed to spec_witness_failed over all
	// catches.
	SpecWitness facts.Fraction `json:"specWitness"`
	// MainlySpecWitness says that the outcome shows a gain and at least two
	// thirds of at least one catch are attributed to spec_witness_failed.
	MainlySpecWitness bool `json:"mainlySpecWitness"`
}

// The two-thirds rule: the spec-witness share is compared as mainlyNumerator
// / mainlyDenominator.
const (
	mainlyNumerator   = 2
	mainlyDenominator = 3
)

// Summarise counts the catches and applies the two-thirds rule under the
// outcome.
func Summarise(catches []Catch, outcome decision.Outcome) Summary {
	summary := Summary{Catches: catches, ByAttribution: map[Attribution]int{}}
	for _, attribution := range Attributions() {
		summary.ByAttribution[attribution] = 0
	}
	for _, caught := range catches {
		summary.ByAttribution[caught.Attribution]++
	}
	witness := summary.ByAttribution[SpecWitness]
	summary.SpecWitness = facts.Fraction{K: witness, N: len(catches)}
	summary.MainlySpecWitness = decision.GainShown(outcome) && len(catches) > 0 &&
		mainlyDenominator*witness >= mainlyNumerator*len(catches)
	return summary
}

// MutationOnly is the runs with every catch attributed to spec_witness_failed
// counted as a run whose final change is accepted and oracle-defective.
func MutationOnly(runs []record.Run) []record.Run {
	recounted := make([]record.Run, len(runs))
	for index, run := range runs {
		if caught, ok := Of(run); ok && caught.Attribution == SpecWitness {
			run.Accepted, run.Defective, run.Escaped = true, true, true
			run.Refusal = record.Refusal{Cause: record.CauseNone, Findings: []facts.Finding{}}
		}
		recounted[index] = run
	}
	return recounted
}
