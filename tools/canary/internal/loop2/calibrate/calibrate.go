// Package calibrate is the calibration of the loop-2 study (ADR 0017,
// "Calibration, readiness and the base-rate stop"): the readiness of each
// coder on a calibration phase, the mechanical selection of the coder with its
// base-rate stop, and the `calibrate` verb that records the selection and the
// reach of the confirmatory design. Judge and Select are pure.
package calibrate

import (
	"errors"
	"fmt"
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/decision"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
)

// Phase is a calibration phase.
type Phase string

// The calibration phases.
const (
	PhaseC1 Phase = "C1"
	PhaseC2 Phase = "C2"
)

// The coders, in the order of preference.
const (
	Sonnet = "claude-sonnet-5-5"
	Haiku  = "claude-haiku-5-5"
)

// Coders lists the coders in the order of preference.
func Coders() []string { return []string{Sonnet, Haiku} }

// The readiness and selection rules.
const (
	// MaxRefusedClean is the most oracle-clean treatment changes a ready coder's
	// gate refuses.
	MaxRefusedClean = 1
	// Loop1FinalTrees is the final trees of loop-1 the P3 replay covers, and
	// ReferencesPerPhase the references of a calibration phase.
	Loop1FinalTrees    = 36
	ReferencesPerPhase = 8
	// MinControlEscapes is the control escapes a coder needs on C1 to be chosen
	// there.
	MinControlEscapes = 5
)

// PositiveControlShare is the least share of held-out violations a ready
// coder's gate refuses with a finding naming the violated criterion.
var PositiveControlShare = facts.Fraction{K: 3, N: 4}

// Run is one calibration run.
type Run struct {
	Run       string       `json:"run"`
	Task      string       `json:"task"`
	Arm       string       `json:"arm"`
	Accepted  bool         `json:"accepted"`
	Defective bool         `json:"defective"`
	Refusal   record.Cause `json:"refusal"`
}

// Violation is one held-out violation of a calibration task under the
// treatment gate on the coder's model.
type Violation struct {
	Task             string `json:"task"`
	Violation        string `json:"violation"`
	Criterion        string `json:"criterion"`
	FixtureTestsPass bool   `json:"fixtureTestsPass"`
	Refused          bool   `json:"refused"`
	NamesCriterion   bool   `json:"namesCriterion"`
}

// Reference is one calibration reference under the P3 replay.
type Reference struct {
	Task    string `json:"task"`
	Firings int    `json:"firings"`
	Passes  bool   `json:"passes"`
}

// Replay is the P3 replay: spec_witness_failed's firings on loop-1's final
// trees and on each reference, and whether each reference with its tests
// passes the treatment gate.
type Replay struct {
	Loop1Trees   int         `json:"loop1Trees"`
	Loop1Firings int         `json:"loop1Firings"`
	References   []Reference `json:"references"`
}

// Entry is one coder's results on one calibration phase.
type Entry struct {
	Coder           string      `json:"coder"`
	Phase           Phase       `json:"phase"`
	Runs            []Run       `json:"runs"`
	PositiveControl []Violation `json:"positiveControl"`
	Replay          Replay      `json:"replay"`
}

// Calibration is calibration.json.
type Calibration struct {
	Coders []Entry `json:"coders"`
}

// entry returns the coder's entry on the phase.
func (c Calibration) entry(coder string, phase Phase) (Entry, bool) {
	at := slices.IndexFunc(c.Coders, func(e Entry) bool { return e.Coder == coder && e.Phase == phase })
	if at < 0 {
		return Entry{}, false
	}
	return c.Coders[at], true
}

func (c Calibration) has(phase Phase) bool {
	return slices.ContainsFunc(c.Coders, func(e Entry) bool { return e.Phase == phase })
}

// The reasons a coder is not ready.
const (
	ReasonRefusedClean    = "the gate refuses more than 1 oracle-clean treatment change"
	ReasonPendingRefusal  = "the gate refuses an oracle-clean treatment change for stale or pending proofs alone"
	ReasonPositiveControl = "the gate refuses fewer than 3/4 of the held-out violations, or there is none"
	ReasonLoop1Replay     = "spec_witness_failed fires on loop-1's final trees, or the replay does not cover all 36"
	ReasonReferences      = "spec_witness_failed fires on a reference, or not every one of the 8 references passes"
)

// Readiness is a coder's readiness on a calibration phase, with what it reads.
type Readiness struct {
	Coder string `json:"coder"`
	Phase Phase  `json:"phase"`
	// RefusedClean is the refused oracle-clean treatment changes over the
	// oracle-clean treatment changes; PendingRefusals those refused for stale
	// or pending proofs alone.
	RefusedClean    facts.Fraction `json:"refusedClean"`
	PendingRefusals int            `json:"pendingRefusals"`
	// PositiveControl is the violations the fixture's tests pass that the gate
	// refuses with a finding naming the criterion, over those violations.
	PositiveControl facts.Fraction `json:"positiveControl"`
	Loop1Trees      int            `json:"loop1Trees"`
	Loop1Firings    int            `json:"loop1Firings"`
	// ReferenceFirings sums the firings on the references; ReferencesPassing
	// is the references that pass over ReferencesPerPhase.
	ReferenceFirings  int            `json:"referenceFirings"`
	ReferencesPassing facts.Fraction `json:"referencesPassing"`
	// ControlEscapes is the control runs whose change is accepted and
	// oracle-defective over the control runs.
	ControlEscapes facts.Fraction `json:"controlEscapes"`
	Ready          bool           `json:"ready"`
	Reasons        []string       `json:"reasons"`
}

// Judge judges a coder's readiness on its entry.
func Judge(e Entry) Readiness {
	r := Readiness{Coder: e.Coder, Phase: e.Phase, Loop1Trees: e.Replay.Loop1Trees, Loop1Firings: e.Replay.Loop1Firings, Reasons: []string{}}
	var clean, refused, controls, escapes int
	for _, run := range e.Runs {
		switch run.Arm {
		case analyze.ArmTreatment:
			if run.Defective {
				continue
			}
			clean++
			if !run.Accepted {
				refused++
				if run.Refusal == record.CausePending {
					r.PendingRefusals++
				}
			}
		case analyze.ArmControl:
			controls++
			if run.Accepted && run.Defective {
				escapes++
			}
		}
	}
	r.RefusedClean = facts.Fraction{K: refused, N: clean}
	r.ControlEscapes = facts.Fraction{K: escapes, N: controls}
	var eligible, caught int
	for _, violation := range e.PositiveControl {
		if !violation.FixtureTestsPass {
			continue
		}
		eligible++
		if violation.Refused && violation.NamesCriterion {
			caught++
		}
	}
	r.PositiveControl = facts.Fraction{K: caught, N: eligible}
	passing := 0
	for _, reference := range e.Replay.References {
		r.ReferenceFirings += reference.Firings
		if reference.Passes {
			passing++
		}
	}
	r.ReferencesPassing = facts.Fraction{K: passing, N: ReferencesPerPhase}
	for _, check := range []struct {
		fails  bool
		reason string
	}{
		{refused > MaxRefusedClean, ReasonRefusedClean},
		{r.PendingRefusals > 0, ReasonPendingRefusal},
		{eligible == 0 || r.PositiveControl.Rat().Cmp(PositiveControlShare.Rat()) < 0, ReasonPositiveControl},
		{r.Loop1Firings > 0 || r.Loop1Trees != Loop1FinalTrees, ReasonLoop1Replay},
		{r.ReferenceFirings > 0 || len(e.Replay.References) != ReferencesPerPhase || passing != ReferencesPerPhase, ReasonReferences},
	} {
		if check.fails {
			r.Reasons = append(r.Reasons, check.reason)
		}
	}
	r.Ready = len(r.Reasons) == 0
	return r
}

// Decision is what the calibration decides.
type Decision string

// The decisions of the calibration.
const (
	DecisionSelected      Decision = "selected"
	DecisionCallC2        Decision = "call C2"
	DecisionBaseRateStop  Decision = "base-rate stop"
	DecisionReturnToFixes Decision = "return to the product fixes"
)

// Selection is the calibration's decision with every readiness it read. A
// selected coder comes with its pooled control escapes over its pooled control
// runs, which the reach reads; the base-rate stop carries its outcome.
type Selection struct {
	Decision  Decision                  `json:"decision"`
	Coder     string                    `json:"coder,omitempty"`
	Outcome   decision.Outcome          `json:"outcome,omitempty"`
	Pooled    map[string]facts.Fraction `json:"pooled"`
	Readiness []Readiness               `json:"readiness"`
}

// Errors of the selection.
var (
	ErrMissingEntry = errors.New("calibration.json lacks a coder's entry")
	ErrUnexpectedC2 = errors.New("calibration.json holds C2 while C1 decides without it")
)

// Select selects the coder: on C1 alone, then, when C1 calls for it, on C2.
func Select(c Calibration) (Selection, error) {
	first, err := judgePhase(c, PhaseC1)
	if err != nil {
		return Selection{}, err
	}
	selection := Selection{Pooled: map[string]facts.Fraction{}, Readiness: first}
	for _, r := range first {
		selection.Pooled[r.Coder] = r.ControlEscapes
	}
	selection.Decision, selection.Coder = selectOnC1(first)
	if selection.Decision != DecisionCallC2 {
		if c.has(PhaseC2) {
			return Selection{}, ErrUnexpectedC2
		}
		return selection, nil
	}
	if !c.has(PhaseC2) {
		return selection, nil
	}
	second, err := judgePhase(c, PhaseC2)
	if err != nil {
		return Selection{}, err
	}
	selection.Readiness = append(selection.Readiness, second...)
	for _, r := range second {
		pooled := selection.Pooled[r.Coder]
		selection.Pooled[r.Coder] = facts.Fraction{K: pooled.K + r.ControlEscapes.K, N: pooled.N + r.ControlEscapes.N}
	}
	selection.Decision, selection.Coder = selectOnC2(second, selection.Pooled)
	if selection.Decision == DecisionBaseRateStop {
		selection.Outcome = decision.NoUsefulGain
	}
	return selection, nil
}

func judgePhase(c Calibration, phase Phase) ([]Readiness, error) {
	var judged []Readiness
	for _, coder := range Coders() {
		e, found := c.entry(coder, phase)
		if !found {
			return nil, fmt.Errorf("%w: %s on %s", ErrMissingEntry, coder, phase)
		}
		judged = append(judged, Judge(e))
	}
	return judged, nil
}

func anyReady(judged []Readiness) bool {
	return slices.ContainsFunc(judged, func(r Readiness) bool { return r.Ready })
}

// selectOnC1 applies the rules after C1: no ready coder returns to the product
// fixes; the first ready coder in the order of preference with at least
// MinControlEscapes control escapes is chosen; otherwise C2 is called.
func selectOnC1(judged []Readiness) (Decision, string) {
	if !anyReady(judged) {
		return DecisionReturnToFixes, ""
	}
	for _, r := range judged {
		if r.Ready && r.ControlEscapes.K >= MinControlEscapes {
			return DecisionSelected, r.Coder
		}
	}
	return DecisionCallC2, ""
}

// selectOnC2 applies the rules after C2: no coder ready on C2 returns to the
// product fixes; every ready coder without a pooled control escape stops the
// programme; otherwise the ready coder with the most pooled control escapes is
// chosen, the first in the order of preference on a tie.
func selectOnC2(judged []Readiness, pooled map[string]facts.Fraction) (Decision, string) {
	if !anyReady(judged) {
		return DecisionReturnToFixes, ""
	}
	best, most := "", 0
	for _, r := range judged {
		if r.Ready && pooled[r.Coder].K > most {
			best, most = r.Coder, pooled[r.Coder].K
		}
	}
	if best == "" {
		return DecisionBaseRateStop, ""
	}
	return DecisionSelected, best
}
