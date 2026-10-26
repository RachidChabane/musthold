package scan

import (
	"context"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/musthold/format"
)

// seedBase writes the seed of a statement in decimal.
const seedBase = 10

// emission is everything a scan executed, to classify and record.
type emission struct {
	commit    string
	base      string
	reference reference
	plan      plan
	drawn     drawn
	executed  []outcome
	request   Request
	// ran are the candidates that ran, and confirmed their classification
	// once its claims are confirmed.
	ran       []Candidate
	confirmed confirmed
	// promises is what a scan that asked its host adds; nil otherwise.
	promises *promiseOutcome
}

// classify classifies every test on what the scan executed.
func (e emission) classify() (vacuity.Input, []Candidate, vacuity.Result, error) {
	input, ran := e.classificationInput()
	result, err := vacuity.Classify(input)
	if err != nil {
		return vacuity.Input{}, nil, vacuity.Result{}, fmt.Errorf("classify the tests: %w", err)
	}
	return input, ran, result, nil
}

// emit records the confirmed classification as the scan statement and picks
// the change each finding shows.
func (s *Service) emit(ctx context.Context, e emission) (Report, error) {
	result := e.confirmed.result
	mutants := scanMutants(e.ran, result)
	predicate := format.Scan{
		Module:     e.request.Module,
		ProducedBy: format.ProducedByLocal,
		Mode:       e.plan.mode,
		Base:       e.base,
		Runner:     string(s.deps.Ecosystem.Name()),
		Seed:       strconv.FormatUint(e.request.Seed, seedBase),
		Partial:    e.partial(),
		Counts:     result.Counts,
		Mutants:    mutants,
		Tests:      result.ScanTests(),
	}
	if e.promises != nil {
		recordPromises(&predicate, e.promises)
	}
	tree, err := s.treeDigest(ctx)
	if err != nil {
		return Report{}, err
	}
	statement := format.NewScanStatement(e.commit, tree, predicate)
	encoded, err := statement.Encode()
	if err != nil {
		return Report{}, fmt.Errorf("encode the scan statement: %w", err)
	}
	findings, err := s.findings(ctx, result, mutants, e.drawn, e.confirmed.killers, e.promises)
	if err != nil {
		return Report{}, err
	}
	promises, err := s.promiseReport(ctx, e.promises, predicate, e.drawn)
	if err != nil {
		return Report{}, err
	}
	return Report{
		Promises:  promises,
		Statement: statement, Encoded: encoded,
		Repository: s.deps.Source.Name(ctx), Commit: e.commit, Runner: s.deps.Ecosystem.Name(),
		Mode: e.plan.mode, Base: e.base, Seed: e.request.Seed,
		Suite:      SuiteCounts{Tests: len(e.reference.cases), Green: len(e.reference.green), Sampled: len(e.plan.sampled)},
		Mutants:    e.mutantCounts(result),
		Candidates: e.drawn.perTest,
		Unproven:   e.unprovenTests(result),
		Findings:   findings,
		Partial:    predicate.Partial,
	}, nil
}

// classificationInput gathers what the domain classifies: the reference runs,
// the sample, the coverage and the mutants that ran with their runs and
// evidence. A mutant that did not build or did not run is left out.
func (e emission) classificationInput() (vacuity.Input, []Candidate) {
	input := vacuity.Input{
		Reference: e.reference.runs,
		Sampled:   e.plan.sampled,
		Coverage:  e.reference.coverage,
		Outcomes:  map[string]map[ecosystem.TestID][]vacuity.Run{},
	}
	var ran []Candidate
	for index, candidate := range e.drawn.candidates {
		executed := e.executed[index]
		if executed.state != stateRan {
			continue
		}
		ran = append(ran, candidate)
		input.Mutants = append(input.Mutants, vacuity.Mutant{
			ID: candidate.ID, Lines: candidate.Lines, Sentinel: candidate.Sentinel, CompiledIdentical: executed.identity,
			Differences: executed.evidence.Differences, WitnessFailures: executed.evidence.WitnessFailures,
		})
		input.Outcomes[candidate.ID] = executed.runs
	}
	return input, ran
}

// partial reports whether the budget left a mutant unrun.
func (e emission) partial() bool {
	return slices.ContainsFunc(e.executed, func(executed outcome) bool { return executed.state == stateNotRun })
}

func (e emission) mutantCounts(result vacuity.Result) MutantCounts {
	counts := MutantCounts{Drawn: len(e.drawn.candidates), AmbiguousUnits: e.drawn.ambiguousUnits, Proven: len(result.Routes), StoppedSentinels: e.stoppedSentinels()}
	for _, executed := range e.executed {
		switch {
		case executed.state == stateNotRun:
			counts.NotRun++
		case executed.state == stateDidNotBuild:
			counts.DidNotBuild++
		case executed.identity:
			counts.Ran++
			counts.CompiledIdentical++
		default:
			counts.Ran++
		}
	}
	return counts
}

// scanMutants writes the mutants that ran, with their edits, lines and routes,
// ordered by id.
func scanMutants(ran []Candidate, result vacuity.Result) []format.ScanMutant {
	mutants := make([]format.ScanMutant, 0, len(ran))
	for _, candidate := range ran {
		mutants = append(mutants, format.ScanMutant{
			ID: candidate.ID, Origin: candidate.Origin, Family: candidate.Family, Sentinel: candidate.Sentinel,
			Edits: candidate.Edits.ScanEdits(), Lines: candidate.Lines, Routes: result.Routes[candidate.ID],
		})
	}
	slices.SortFunc(mutants, func(a, b format.ScanMutant) int { return strings.Compare(a.ID, b.ID) })
	return mutants
}

// treeDigest is the file-set digest of the tracked files of the head commit,
// scan reports aside.
func (s *Service) treeDigest(ctx context.Context) (string, error) {
	tracked, err := s.deps.Source.TrackedFiles(ctx)
	if err != nil {
		return "", fmt.Errorf("list the tracked files: %w", err)
	}
	var files []format.File
	for _, path := range tracked {
		if path == outputDirectory || strings.HasPrefix(path, outputDirectory+"/") {
			continue
		}
		content, err := fs.ReadFile(s.deps.Project, path)
		if err != nil {
			continue
		}
		files = append(files, format.File{Path: path, Content: content})
	}
	digest, err := format.FileSetDigest(files)
	if err != nil {
		return "", fmt.Errorf("digest the tree: %w", err)
	}
	return digest, nil
}

// findings gives each test that verifies nothing, in rank order, with the first
// of its changes that a differential difference proves, else its first change,
// and the route its confirmation upheld.
func (s *Service) findings(ctx context.Context, result vacuity.Result, mutants []format.ScanMutant, drawn drawn, killers map[string][]ecosystem.TestID, promises *promiseOutcome) ([]Finding, error) {
	byID := map[string]format.ScanMutant{}
	for _, mutant := range mutants {
		byID[mutant.ID] = mutant
	}
	var findings []Finding
	for _, verdict := range result.Verdicts {
		if verdict.Class != format.ScanVerifiesNothing {
			continue
		}
		change := shownChange(verdict.Changes, byID)
		mutant := byID[change.Mutant]
		files, err := s.fileChanges(mutant, drawn)
		if err != nil {
			return nil, err
		}
		finding := Finding{
			Rank: len(findings) + 1, Test: verdict.Test, Changes: len(verdict.Changes), NotStarted: len(verdict.NotStarted),
			Mutant: mutant, Site: change.Site, Route: shownRoute(mutant.Routes, killers[mutant.ID]),
			Files: files, Repro: s.deps.Suite.Repro(verdict.Test),
		}
		if finding.Killers, err = s.killersOf(ctx, mutant, promises); err != nil {
			return nil, err
		}
		if finding.Route.Kind == format.ScanRouteWitness && promises != nil {
			finding.Witnesses, finding.Spec = promises.files[finding.Route.Witness], promises.spec
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

// shownChange is the first change a differential difference proves, else the
// first change; changes come ordered by site.
func shownChange(changes []vacuity.Change, mutants map[string]format.ScanMutant) vacuity.Change {
	for _, change := range changes {
		if slices.ContainsFunc(mutants[change.Mutant].Routes, isDifferential) {
			return change
		}
	}
	return changes[0]
}

// shownRoute is the first differential route, else the first kill by a
// killer the confirmation upheld, else the first route.
func shownRoute(routes []format.ScanRoute, confirmedKillers []ecosystem.TestID) format.ScanRoute {
	if index := slices.IndexFunc(routes, isDifferential); index >= 0 {
		return routes[index]
	}
	upheld := func(route format.ScanRoute) bool {
		return route.Kind == format.ScanRouteKilledByTest && slices.Contains(confirmedKillers, ecosystem.TestID(route.Test))
	}
	if index := slices.IndexFunc(routes, upheld); index >= 0 {
		return routes[index]
	}
	return routes[0]
}

func isDifferential(route format.ScanRoute) bool { return route.Kind == format.ScanRouteDifferential }

// fileChanges splices the mutant's edits into the reference sources of the
// files it edits.
func (s *Service) fileChanges(mutant format.ScanMutant, drawn drawn) ([]FileChange, error) {
	edits := mutation.EditSetOf(mutant.Edits)
	var changes []FileChange
	for _, file := range edits.Files() {
		splicer, err := s.deps.Engines.SplicerFor(file)
		if err != nil {
			return nil, err
		}
		before := drawn.sources[file]
		after, err := splicer.SpliceAll(file, before, edits.EditsOf(file))
		if err != nil {
			return nil, fmt.Errorf("splice mutant %s: %w", mutant.ID, err)
		}
		changes = append(changes, FileChange{Path: file, Before: before, After: after})
	}
	return changes, nil
}
