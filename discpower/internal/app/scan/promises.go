package scan

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
)

// PromiseReport is what a scan that asked its host established about the
// promises of the code: what it checked, what it found, and what went
// unchecked and why.
type PromiseReport struct {
	// Checked is set when the host extracted promises, or when the scan had no
	// source to extract them from.
	Checked bool
	// NoSource is set when the scan had no quotable source.
	NoSource bool
	// Promises are the promises the scan checked, in the order it kept them.
	Promises []format.ScanPromise
	// Dropped are the extracted promises whose quote the check refused.
	Dropped []promise.Dropped
	// Unchecked are the kept promises the scan did not check, with why.
	Unchecked []UncheckedPromise
	// Cut are the asks the budget planner did not issue, and Unasked those the
	// emission cap left without a ticket.
	Cut     []promise.Cut
	Unasked []promise.Ask
	// Broken are the broken promises, each with the change that breaks it.
	Broken []BrokenPromise
	// Questions are the witnesses that are questions about their promise.
	Questions []PromiseQuestion
	// ModelMutants counts the model mutants the scan added to its draw.
	ModelMutants int
	// Calls counts the calls the session made to the host, reissues included,
	// out of Budget.
	Calls  int
	Budget int
	// WitnessesUnavailable is set for an ecosystem that writes no witness.
	WitnessesUnavailable bool
	// Truncated are the code views the size bound cut, the generation views
	// in promise order, then the labels view.
	Truncated []TruncatedView
}

// NoPromise reports whether no quotable promise was found: no source, or no
// quote the check kept.
func (r PromiseReport) NoPromise() bool {
	return r.Checked && len(r.Promises) == 0 && len(r.Unchecked) == 0
}

// UncheckedPromise is a kept promise the scan did not check.
type UncheckedPromise struct {
	Promise format.ScanPromise
	Reason  promise.UncheckedReason
}

// BrokenPromise is a promise a mutant breaks: the mutant fails, reproduced, a
// block-bearing witness of the promise and passes the whole suite three runs
// out of three.
type BrokenPromise struct {
	// Rank is its place among the broken promises, from 1.
	Rank    int
	Promise format.ScanPromise
	Mutant  format.ScanMutant
	Witness format.ScanWitness
	// File holds the witnesses laid out with it, as the scan ran them.
	File []format.ScanWitness
	// Site is the first mutated line of the mutant.
	Site  format.ScanLine
	Files []FileChange
	// Killer is the witness, laid out as a test of the project, that fails on
	// the mutant.
	Killer Killer
	// Spec are the promises the witnesses were written against.
	Spec []format.ScanPromise
}

// PromiseQuestion is a witness that is a question about its promise, never a
// finding.
type PromiseQuestion struct {
	Promise format.ScanPromise
	Witness format.ScanWitness
	Reason  promise.QuestionReason
}

// promiseOutcome is what the final stage adds to the statement and the report.
type promiseOutcome struct {
	checked   bool
	promises  []format.ScanPromise
	witnesses []format.ScanWitness
	broken    []format.ScanBrokenPromise
	questions []PromiseQuestion
	labels    promise.Labels
	report    PromiseReport
	// spec are the promises the witnesses were written against: every chosen
	// promise, as the writers and the witness host were told of them.
	spec []format.ScanPromise
	// files are the witnesses of each witness file, by witness id.
	files map[string][]format.ScanWitness
	// units are the units of the code, whose lines a label joins a mutant's
	// site on.
	units []viewUnit
}

// outcome reads what the promises came to: the witnesses with their standing,
// the questions, and the broken promises, each candidate of which runs the
// whole suite three times first.
func (h *hostScan) outcome(ctx context.Context, built reference, drawn drawn, executed []outcome) (*promiseOutcome, error) {
	state := h.run.state
	out := &promiseOutcome{checked: state.checked && !state.extractionFailed, labels: state.labels, files: map[string][]format.ScanWitness{}}
	for _, chosen := range state.choice.Chosen {
		out.spec = append(out.spec, chosen.Promise)
	}
	code, err := h.run.codeOf(ctx)
	if err != nil {
		return nil, err
	}
	out.units = code.units
	var standings []promise.WitnessStanding
	for _, kept := range state.checking {
		out.promises = append(out.promises, kept.Promise)
		for _, record := range state.recordsOf(kept.Promise.ID) {
			standing := record.standing(kept.Promise.ID)
			standings = append(standings, standing)
			scanned := scanWitnessOf(kept.Promise.ID, record, standing)
			out.witnesses = append(out.witnesses, scanned)
			if record.question != "" {
				out.questions = append(out.questions, PromiseQuestion{Promise: kept.Promise, Witness: scanned, Reason: record.question})
			}
		}
	}
	out.groupFiles()
	broken, err := h.brokenPromises(ctx, built, drawn, executed, standings)
	if err != nil {
		return nil, err
	}
	out.broken = broken
	out.report = h.report(out)
	return out, nil
}

// scanWitnessOf is a witness as the statement records it: admitted when it
// passed all six runs on the reference.
func scanWitnessOf(promiseID string, record *witnessRecord, standing promise.WitnessStanding) format.ScanWitness {
	return format.ScanWitness{
		ID: record.witness.ID, Promise: promiseID, Name: record.witness.Name, Imports: record.imports, Code: record.witness.Code,
		Admitted: record.stable, BlockBearing: standing.BlockBearing(scanThreshold()),
	}
}

// groupFiles gathers the witnesses each witness is laid out with: those of
// its promise with the same imports, which one test file holds.
func (o *promiseOutcome) groupFiles() {
	for _, one := range o.witnesses {
		for _, other := range o.witnesses {
			if other.Promise == one.Promise && other.Imports == one.Imports {
				o.files[one.ID] = append(o.files[one.ID], other)
			}
		}
	}
}

// brokenPromises reads the broken promises: each mutant that ran, is no
// sentinel, no test killed and fails a block-bearing witness runs the whole
// suite three times, and breaks a promise when it passes every run.
func (h *hostScan) brokenPromises(ctx context.Context, built reference, drawn drawn, executed []outcome, standings []promise.WitnessStanding) ([]format.ScanBrokenPromise, error) {
	bearing := map[string]bool{}
	for _, standing := range standings {
		if standing.BlockBearing(scanThreshold()) {
			bearing[standing.ID] = true
		}
	}
	var broken []format.ScanBrokenPromise
	for index, candidate := range drawn.candidates {
		ran := executed[index]
		if ran.state != stateRan || candidate.Sentinel || ran.identity || killedByOne(ran.runs) {
			continue
		}
		outcomes := h.evidence.outcomesOf(candidate.ID)
		if !failsAny(outcomes, bearing) {
			continue
		}
		suite, err := h.run.service.wholeSuite(ctx, built, candidate)
		if err != nil {
			return nil, err
		}
		broken = append(broken, promise.BrokenPromises(promise.MutantEvidence{ID: candidate.ID, Witness: outcomes, Suite: suite}, standings, scanThreshold())...)
	}
	slices.SortFunc(broken, func(a, b format.ScanBrokenPromise) int {
		return cmp.Or(strings.Compare(a.Promise, b.Promise), strings.Compare(a.Mutant, b.Mutant), strings.Compare(a.Witness, b.Witness))
	})
	return broken, nil
}

// failsAny reports whether one of the witnesses failed, reproduced.
func failsAny(outcomes map[string]witness.Outcome, witnesses map[string]bool) bool {
	for id := range witnesses {
		if outcomes[id] == witness.OutcomeFailed {
			return true
		}
	}
	return false
}

// report is the part of the promise report the stages decide; the broken
// promises are added once the statement's mutants are known.
func (h *hostScan) report(out *promiseOutcome) PromiseReport {
	state, asker := h.run.state, h.run.asker
	report := PromiseReport{
		Checked: out.checked, NoSource: state.noSources, Promises: out.promises, Dropped: state.quotes.Dropped,
		Cut: asker.cuts, Unasked: asker.unasked, Questions: out.questions, ModelMutants: len(h.models.candidates),
		Calls: h.calls, Budget: promise.Budget, WitnessesUnavailable: state.witnessesUnavailable, Truncated: state.truncated,
	}
	byID := map[string]format.ScanPromise{}
	for _, kept := range state.choice.Chosen {
		byID[kept.Promise.ID] = kept.Promise
	}
	for _, unchecked := range state.choice.Unchecked {
		report.Unchecked = append(report.Unchecked, UncheckedPromise{Promise: promiseOf(state.quotes.Kept, unchecked.Promise), Reason: unchecked.Reason})
	}
	checking := state.checkingIDs()
	for _, kept := range state.choice.Chosen {
		if !slices.Contains(checking, kept.Promise.ID) {
			report.Unchecked = append(report.Unchecked, UncheckedPromise{Promise: byID[kept.Promise.ID], Reason: promise.UncheckedBudget})
		}
	}
	return report
}

// promiseOf finds a kept promise by id.
func promiseOf(kept []promise.Kept, id string) format.ScanPromise {
	for _, one := range kept {
		if one.Promise.ID == id {
			return one.Promise
		}
	}
	return format.ScanPromise{ID: id}
}

// recordPromises writes the promises into the statement: the checked
// promises, every witness with its standing, the broken promises and the
// questions, each in id order, and the labels on the tests, which change no
// verdict.
func recordPromises(predicate *format.Scan, out *promiseOutcome) {
	predicate.PromisesChecked = out.checked
	predicate.Promises = slices.SortedFunc(slices.Values(out.promises), func(a, b format.ScanPromise) int { return strings.Compare(a.ID, b.ID) })
	predicate.Witnesses = slices.SortedFunc(slices.Values(out.witnesses), func(a, b format.ScanWitness) int { return strings.Compare(a.ID, b.ID) })
	predicate.BrokenPromises = out.broken
	for _, question := range out.questions {
		predicate.Questions = append(predicate.Questions, format.ScanQuestion{Promise: question.Promise.ID, Witness: question.Witness.ID})
	}
	slices.SortFunc(predicate.Questions, func(a, b format.ScanQuestion) int {
		return cmp.Or(strings.Compare(a.Promise, b.Promise), strings.Compare(a.Witness, b.Witness))
	})
	functionOf := map[string]string{}
	for _, mutant := range predicate.Mutants {
		if unit, found := unitAt(out.units, firstLine(mutant)); found {
			functionOf[mutant.ID] = unit.key()
		}
	}
	for index, test := range predicate.Tests {
		predicate.Tests[index] = promise.Label(test, out.labels, functionOf)
	}
}

// unitAt is the innermost unit whose lines hold the line: the function a
// change at that site lies in.
func unitAt(units []viewUnit, line format.ScanLine) (viewUnit, bool) {
	var found viewUnit
	held := false
	for _, unit := range units {
		if unit.file != line.File || !slices.Contains(unit.lines, line.Line) {
			continue
		}
		if !held || len(unit.lines) < len(found.lines) {
			found, held = unit, true
		}
	}
	return found, held
}

// witnessTestID is the id a scan gives the test a witness was laid out as: its
// file and its name, as the suite's ids are written.
func witnessTestID(test ecosystem.BoundTest) ecosystem.TestID {
	return ecosystem.TestID(test.File + unitKeySeparator + test.Name)
}
