// Package recordtest builds loop-2 run records for the tests of the packages
// that read them.
package recordtest

import (
	"fmt"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
)

// The arms of the study, as loop-1 names them.
const (
	Control   = analyze.ArmControl
	Treatment = analyze.ArmTreatment
)

// Option sets one property of a run.
type Option func(*record.Run)

// Run builds a counted run of the task and arm whose final change is accepted
// and oracle-clean, then applies the options in order.
func Run(task, arm string, repetition int, options ...Option) record.Run {
	run := record.Run{
		Facts: facts.Facts{
			Run: fmt.Sprintf("%s-%s-%d", task, arm, repetition), Task: task, Arm: arm, Repetition: repetition,
			Accepted: true, Violated: []string{}, Blocks: []facts.Block{},
			BlockCounts: map[facts.BlockKind]int{}, FindingCodes: map[string]int{}, Hits: map[study.Detector]int{},
		},
		Refusal: record.Refusal{Cause: record.CauseNone, Findings: []facts.Finding{}},
	}
	for _, option := range options {
		option(&run)
	}
	run.Escaped = run.Accepted && run.Defective
	return run
}

// Defective makes the final change oracle-defective on the criteria.
func Defective(criteria ...string) Option {
	return func(run *record.Run) { run.Defective, run.Violated = true, criteria }
}

// Refused refuses the final change for the cause with the gate's findings.
func Refused(cause record.Cause, findings ...facts.Finding) Option {
	return func(run *record.Run) {
		run.Accepted = false
		run.Refusal = record.Refusal{Cause: cause, Findings: append([]facts.Finding{}, findings...)}
	}
}

// FirstStop grades the tree at the first end-of-turn call.
func FirstStop(defective bool, violated ...string) Option {
	return func(run *record.Run) {
		run.FirstStop = &record.Grade{Defective: defective, Violated: append([]string{}, violated...)}
	}
}

// FindingBlock adds a finding block with the findings, corrected or left.
func FindingBlock(corrected bool, findings ...facts.Finding) Option {
	return func(run *record.Run) {
		block := facts.Block{Call: len(run.Blocks) + 1, Kind: facts.KindFinding, Findings: findings, Corrected: corrected, Left: !corrected}
		run.Blocks = append(run.Blocks, block)
		run.BlockCounts[facts.KindFinding]++
		for _, finding := range findings {
			run.FindingCodes[finding.Code]++
		}
	}
}

// Adjudicated gives an escape its adjudication, disputed or upheld.
func Adjudicated(disputed bool) Option {
	return func(run *record.Run) {
		criterion := ""
		if len(run.Violated) > 0 {
			criterion = run.Violated[0]
		}
		run.Adjudication = &record.Adjudication{Criterion: criterion, Sentence: "The rule of " + criterion + ".", Judges: []record.Verdict{
			{Judge: record.JudgeTerra, Defective: true}, {Judge: record.JudgeOpus, Defective: !disputed},
		}}
	}
}

// Finding is a blocking finding.
func Finding(code, criterion, module string) facts.Finding {
	return facts.Finding{Code: code, Criterion: criterion, Module: module}
}
