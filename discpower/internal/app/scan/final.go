package scan

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
	"github.com/RachidChabane/musthold/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
)

// Model mutants of a scan.
const (
	// ModelMutantCap is the most model mutants a scan adds to its draw.
	ModelMutantCap = 20
	// contradictionNoUsableMutant is the ground on which a generation answer is
	// rejected: none of its mutants splices, changes the code and builds.
	contradictionNoUsableMutant = "no_usable_mutant"
)

// final is the last stage, run in the first invocation where nothing waits:
// it splices and builds the model mutants first, and a generation answer none
// of whose mutants is usable is rejected and asked again before any suite
// runs. Then, in this one process, the scan runs as a scan without a host
// does, with the model mutants after the operator mutants and the witnesses as
// one more source of evidence, and records the promises.
func (r *hostRun) final(ctx context.Context, started time.Time) (finish, error) {
	built, err := r.reference(ctx)
	if err != nil {
		return finish{}, err
	}
	models, settled, err := r.modelMutants(ctx, built)
	if err != nil || !settled {
		return finish{}, err
	}
	calls, err := r.asker.emissions()
	if err != nil {
		return finish{}, err
	}
	host := &hostScan{run: r, models: models, calls: calls}
	report, err := r.service.scanFrom(ctx, scanInput{
		started: started, copied: r.builtAt, commit: r.commit, base: r.change.base, request: r.request, reference: *built, host: host,
	})
	if err != nil {
		return finish{}, err
	}
	return finish{done: true, report: report}, nil
}

// modelSet is the model mutants of a scan with the reference bytes of every
// file they edit.
type modelSet struct {
	candidates []Candidate
	sources    map[string][]byte
}

// modelMutants reads every answered generation ticket into model mutants, at
// most ModelMutantCap in ticket order. A ticket none of whose proposals is
// usable is contradicted and asked again; the stage is settled when no ticket
// waits.
func (r *hostRun) modelMutants(ctx context.Context, built *reference) (modelSet, bool, error) {
	set := modelSet{sources: map[string][]byte{}}
	seen := map[string]bool{}
	settled := true
	for _, ticket := range r.state.generation {
		if !answered(ticket.answer) {
			continue
		}
		usable, err := r.intake(ctx, built, ticket.answer.Votes[0].Output, &set, seen)
		if err != nil {
			return modelSet{}, false, err
		}
		if usable > 0 {
			if err := r.validate(*ticket.answer); err != nil {
				return modelSet{}, false, err
			}
			continue
		}
		done, err := r.contradictVotes(ticket.ask, ticket.answer.Votes, contradictionNoUsableMutant)
		if err != nil {
			return modelSet{}, false, err
		}
		settled = settled && done
	}
	return set, settled, nil
}

// intake adds the usable proposals of one generation answer to the model
// mutants and counts them; a proposal already drawn, or past the cap, is
// usable but adds nothing.
func (r *hostRun) intake(ctx context.Context, built *reference, output []byte, set *modelSet, seen map[string]bool) (int, error) {
	generated, err := prompt.ParseGeneration(output)
	if err != nil {
		return 0, nil
	}
	usable := 0
	for _, proposal := range generated {
		candidate, files, ok, err := r.modelCandidate(ctx, built, unitTexts(proposal.Edits))
		if err != nil {
			return 0, err
		}
		if !ok {
			continue
		}
		usable++
		if seen[candidate.ID] || len(set.candidates) == ModelMutantCap {
			continue
		}
		seen[candidate.ID] = true
		set.candidates = append(set.candidates, candidate)
		maps.Copy(set.sources, files)
	}
	return usable, nil
}

// lineBreaks are the characters a unit's text never ends with.
const lineBreaks = "\r\n"

// unitTexts reads each proposed source as the text of its unit, which ends
// with its last character: the line breaks a model ends its code with are no
// part of the unit, and a splice of them would never match it.
func unitTexts(edits mutation.EditSet) mutation.EditSet {
	trimmed := mutation.EditSet{Edits: make([]mutation.Edit, len(edits.Edits))}
	for index, edit := range edits.Edits {
		edit.NewSource = strings.TrimRight(edit.NewSource, lineBreaks)
		trimmed.Edits[index] = edit
	}
	return trimmed
}

// modelCandidate checks a proposed edit set: known target kinds, a file a scan
// may mutate for each edit, a splice that changes lines none of which calls a
// sink, and a mutant that builds. It gives the mutant with origin model and
// the reference bytes of the files it edits.
func (r *hostRun) modelCandidate(ctx context.Context, built *reference, edits mutation.EditSet) (Candidate, map[string][]byte, bool, error) {
	if edits.Validate() != nil {
		return Candidate{}, nil, false, nil
	}
	code, err := r.codeOf(ctx)
	if err != nil {
		return Candidate{}, nil, false, err
	}
	files := map[string][]byte{}
	var lines []vacuity.Line
	for _, file := range edits.Files() {
		index := slices.IndexFunc(code.files, func(source mutation.SourceFile) bool { return source.Path == file })
		if index < 0 {
			return Candidate{}, nil, false, nil
		}
		mutated, ok := r.splicedLines(code.files[index], edits)
		if !ok {
			return Candidate{}, nil, false, nil
		}
		files[file] = code.files[index].Content
		lines = append(lines, mutated...)
	}
	if slices.ContainsFunc(edits.Edits, func(edit mutation.Edit) bool { return edit.Kind() == mutation.TargetModuleAddition }) {
		return Candidate{}, nil, false, nil
	}
	status, err := built.executor.Mutate(ctx, built.referenceOf(), edits, func(context.Context, ecosystem.Workspace) error { return nil })
	if err != nil || !status.Built {
		return Candidate{}, nil, false, ignorePolicy(err)
	}
	id, err := format.ScanEditsDigest(edits.ScanEdits())
	if err != nil {
		return Candidate{}, nil, false, fmt.Errorf("digest a model mutant: %w", err)
	}
	slices.SortFunc(lines, format.CompareScanLines)
	return Candidate{ID: id, Edits: edits, Origin: format.ScanOriginModel, Lines: slices.Compact(lines)}, files, true, nil
}

// splicedLines splices the edits of one file into its reference bytes and
// gives the lines they mutate; a splice that fails, changes nothing or touches
// a line that calls a sink is unusable.
func (r *hostRun) splicedLines(file mutation.SourceFile, edits mutation.EditSet) ([]vacuity.Line, bool) {
	splicer, err := r.service.deps.Engines.SplicerFor(file.Path)
	if err != nil {
		return nil, false
	}
	after, err := splicer.SpliceAll(file.Path, file.Content, edits.EditsOf(file.Path))
	if err != nil {
		return nil, false
	}
	lines := scanscope.MutatedLines(file.Path, file.Content, after)
	if len(lines) == 0 || slices.ContainsFunc(lines, func(line vacuity.Line) bool {
		return scanscope.SinkLine(scanscope.LineText(file.Content, line.Line))
	}) {
		return nil, false
	}
	return lines, true
}

// ignorePolicy reads a path the policy refuses as an unusable proposal, not a
// failure of the scan.
func ignorePolicy(err error) error {
	if err == nil || errors.Is(err, mutation.ErrPathRefused) {
		return nil
	}
	return err
}

// hostScan is what a scan that asks a host adds to the phases of a scan: its
// model mutants, its witnesses as evidence, and its promises.
type hostScan struct {
	run    *hostRun
	models modelSet
	// calls are the emissions of the session: the model calls it used.
	calls int
	// evidence runs the stable witnesses on each mutant no test killed.
	evidence *witnessEvidence
}

// withModels adds the model mutants to the draw, after the operator mutants;
// one the operators already drew stays theirs.
func (h *hostScan) withModels(drawn drawn) drawn {
	var drawOrder []string
	for _, index := range drawn.order {
		if !drawn.candidates[index].Sentinel {
			drawOrder = append(drawOrder, drawn.candidates[index].ID)
		}
	}
	known := map[string]bool{}
	for _, candidate := range drawn.candidates {
		known[candidate.ID] = true
	}
	sources := maps.Clone(drawn.sources)
	for _, candidate := range h.models.candidates {
		if known[candidate.ID] {
			continue
		}
		known[candidate.ID] = true
		drawn.candidates = append(drawn.candidates, candidate)
		drawOrder = append(drawOrder, candidate.ID)
	}
	for file, content := range h.models.sources {
		if _, held := sources[file]; !held {
			sources[file] = content
		}
	}
	drawn.sources = sources
	slices.SortFunc(drawn.candidates, func(a, b Candidate) int { return strings.Compare(a.ID, b.ID) })
	drawn.order = startOrder(drawn.candidates, drawOrder)
	return drawn
}

// withWitnesses runs each admitted set three more times on the reference, with
// no memo, so that a witness backs a claim only when all six runs passed, and
// adds the stable witnesses to the evidence a mutant no test killed is
// searched for.
func (h *hostScan) withWitnesses(ctx context.Context, built reference, sources []EvidenceSource) ([]EvidenceSource, error) {
	state := h.run.state
	if state.witnessesUnavailable || len(state.witnesses) == 0 {
		return sources, nil
	}
	runner := h.run.service.deps.Witnesses.For(WitnessReference{Executor: built.executor, Reference: built.referenceOf()}, nil, h.run.suiteKey())
	h.evidence = &witnessEvidence{runner: runner, outcomes: map[string]map[string]witness.Outcome{}}
	for _, kept := range state.checking {
		for _, set := range state.runnableSets(kept.Promise.ID, admitted) {
			runs, err := runner.Repeat(ctx, emptyEdits, set)
			if err != nil {
				return nil, fmt.Errorf("run the witnesses of promise %s on the reference again: %w", kept.Promise.ID, err)
			}
			h.confirmSet(kept.Promise.ID, set, runs)
		}
		h.evidence.sets = append(h.evidence.sets, state.runnableSets(kept.Promise.ID, stable)...)
	}
	return append(sources, h.evidence), nil
}

// confirmSet records the final runs of the admitted witnesses of a set.
func (h *hostScan) confirmSet(promiseID string, set witness.Set, runs map[string][]witness.Outcome) {
	own := h.run.state.witnesses[promiseID]
	for _, w := range set.Witnesses {
		record := own.records[w.ID]
		if !record.admitted || record.final != nil {
			continue
		}
		record.final = runs[w.ID]
		decision := promise.Confirm(record.admission, record.final)
		record.stable = decision.Holds
		if !decision.Holds {
			record.question = decision.Question
		}
		if record.stable {
			h.evidence.stable = append(h.evidence.stable, w.ID)
		}
	}
}

// stable reports whether a witness backs claims: admitted and passing all six
// runs.
func stable(record *witnessRecord) bool { return record != nil && record.stable }

// witnessEvidence is the EvidenceSource of the witnesses: it runs the sets that
// hold a stable witness on each mutant no test killed, in a workspace of their
// own, and reports the stable witnesses that fail it, reproduced.
type witnessEvidence struct {
	runner WitnessRunner
	sets   []witness.Set
	stable []string
	mutex  sync.Mutex
	// outcomes are the outcomes of every witness on each mutant, by mutant id.
	outcomes map[string]map[string]witness.Outcome
}

var _ EvidenceSource = (*witnessEvidence)(nil)

// Gather runs the witnesses on the mutant.
func (w *witnessEvidence) Gather(ctx context.Context, candidate Candidate, _ ecosystem.Workspace, _ map[ecosystem.TestID][]vacuity.Run) (Evidence, error) {
	if len(w.sets) == 0 {
		return Evidence{}, nil
	}
	outcomes, err := w.runner.OnMutant(ctx, candidate.ID, candidate.Edits, w.sets)
	if err != nil {
		return Evidence{}, fmt.Errorf("run the witnesses on mutant %s: %w", candidate.ID, err)
	}
	w.mutex.Lock()
	w.outcomes[candidate.ID] = outcomes
	w.mutex.Unlock()
	var evidence Evidence
	for _, id := range slices.Sorted(slices.Values(w.stable)) {
		if outcomes[id] == witness.OutcomeFailed {
			evidence.WitnessFailures = append(evidence.WitnessFailures, vacuity.WitnessFailure{Witness: id, Admitted: true, Blind: true, Reproduced: true})
		}
	}
	return evidence, nil
}

// outcomesOf are the outcomes of the witnesses on a mutant.
func (w *witnessEvidence) outcomesOf(mutant string) map[string]witness.Outcome {
	if w == nil {
		return nil
	}
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.outcomes[mutant]
}

// wholeSuite runs the whole suite three times on the mutant: each run passes
// when every test green on the reference passed it. A mutant that does not
// build passes no run.
func (s *Service) wholeSuite(ctx context.Context, built reference, candidate Candidate) ([]witness.Outcome, error) {
	var runs []witness.Outcome
	status, err := built.executor.Mutate(ctx, built.referenceOf(), candidate.Edits, func(ctx context.Context, mutant ecosystem.Workspace) error {
		ran, err := s.deps.Suite.Run(ctx, mutant, built.toolchain, ports.ScanRunRequest{Runs: vacuity.RunsPerOutcome, Timeout: ReferenceTimeout})
		if err != nil {
			return err
		}
		runs = suiteRuns(ran.Runs, built.green)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("run the whole suite on mutant %s: %w", candidate.ID, err)
	}
	if !status.Built {
		return nil, nil
	}
	return runs, nil
}

// suiteRuns reads the runs of the whole suite: run i passes when every green
// test passed its run i.
func suiteRuns(runs map[ecosystem.TestID][]vacuity.Run, green []ecosystem.TestID) []witness.Outcome {
	outcomes := make([]witness.Outcome, vacuity.RunsPerOutcome)
	for index := range outcomes {
		outcomes[index] = witness.OutcomePassed
		for _, test := range green {
			if index >= len(runs[test]) || runs[test][index] != vacuity.RunPassed {
				outcomes[index] = witness.OutcomeFailed
				break
			}
		}
	}
	return outcomes
}
