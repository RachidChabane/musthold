package scan

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
	"github.com/RachidChabane/musthold/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/musthold/format"
)

// Candidate is one mutant a scan runs: its edits, where they mutate and who
// drew it.
type Candidate struct {
	// ID is the digest of the edits, the mutant id of the scan statement.
	ID     string
	Edits  mutation.EditSet
	Origin format.ScanOrigin
	// Family is the operator family of an operator mutant.
	Family string
	// Sentinel is set for a mutant that keeps behaviour by construction.
	Sentinel bool
	// Lines are the lines the mutant mutates in the reference.
	Lines []vacuity.Line
}

// drawn is the mutants of a scan with the sources they were drawn from.
type drawn struct {
	candidates []Candidate
	// sources holds the reference content of every file a mutant edits.
	sources map[string][]byte
	// perTest counts, for each sampled test with an operator mutant among its
	// lines, the drawn operator mutants with a mutated line among them.
	perTest map[ecosystem.TestID]int
	// order lists the candidates in the order they start: the sentinels
	// first, so that no kill goes unguarded when a budget cuts the scan, then
	// the operator mutants in the order the allocation drew them.
	order []int
	// ambiguousUnits counts the units the draw left out because their file
	// defines their name more than once.
	ambiguousUnits int
}

// draw generates the operator mutants and sentinels of the files the targets
// lie in, keeps those whose mutated lines are targets and call no sink,
// allocates the operator mutants over the lines of the sampled tests and keeps
// SentinelCap sentinels, stratified by file.
func (s *Service) draw(ref reference, plan plan, request Request) (drawn, error) {
	files, err := s.mutableFiles(ref.layout, plan.targets.Files())
	if err != nil {
		return drawn{}, err
	}
	sources := map[string][]byte{}
	for _, file := range files {
		sources[file.Path] = file.Content
	}
	lines := sampledLines(ref, plan)
	generated, err := operatorgen.NewGenerator(s.deps.Engines).Generate(operatorgen.Request{
		Files:  files,
		Config: operator.Config{Seed: scanscope.DrawSeed(request.Seed)},
		Admit: func(file mutation.SourceFile, site operator.Site) bool {
			return admits(plan.targets, file, site)
		},
		Selector: allocationSelector{sources: sources, lines: lines, budget: request.Mutants, seed: request.Seed},
	})
	if err != nil {
		return drawn{}, fmt.Errorf("draw the mutants: %w", err)
	}
	result := drawn{sources: sources, ambiguousUnits: generated.Stats.AmbiguousUnits}
	sentinels := scanscope.KeepSentinels(generated.Sentinels, scanscope.SentinelCap, request.Seed)
	seen := map[string]bool{}
	var drawOrder []string
	for _, mutant := range slices.Concat(generated.Operator, sentinels) {
		candidate, err := candidateOf(mutant, sources[mutant.Site.File])
		if err != nil {
			return drawn{}, err
		}
		if !seen[candidate.ID] && len(candidate.Lines) > 0 {
			seen[candidate.ID] = true
			result.candidates = append(result.candidates, candidate)
			drawOrder = append(drawOrder, candidate.ID)
		}
	}
	slices.SortFunc(result.candidates, func(a, b Candidate) int { return strings.Compare(a.ID, b.ID) })
	result.perTest = operatorMutantsPerTest(result.candidates, lines)
	result.order = startOrder(result.candidates, drawOrder)
	return result, nil
}

// startOrder lists the candidates, by index, sentinels first in id order, then
// the operator mutants in the order they were drawn.
func startOrder(candidates []Candidate, drawOrder []string) []int {
	index := make(map[string]int, len(candidates))
	var order []int
	for position, candidate := range candidates {
		index[candidate.ID] = position
		if candidate.Sentinel {
			order = append(order, position)
		}
	}
	for _, id := range drawOrder {
		if position := index[id]; !candidates[position].Sentinel {
			order = append(order, position)
		}
	}
	return order
}

// operatorMutantsPerTest counts, for each test, the operator mutants with a
// mutated line among its lines; a test with none is left out.
func operatorMutantsPerTest(candidates []Candidate, lines map[ecosystem.TestID]scanscope.LineSet) map[ecosystem.TestID]int {
	counts := map[ecosystem.TestID]int{}
	for test, set := range lines {
		for _, candidate := range candidates {
			if !candidate.Sentinel && set.Intersects(candidate.Lines) {
				counts[test]++
			}
		}
	}
	return counts
}

// sampledLines are the lines each sampled test owns that a draw may mutate:
// the allocation spends a test's quota only on lines where a survival can
// count against it.
func sampledLines(ref reference, plan plan) map[ecosystem.TestID]scanscope.LineSet {
	lines := make(map[ecosystem.TestID]scanscope.LineSet, len(plan.sampled))
	for _, test := range plan.sampled {
		set := scanscope.LineSet{}
		for _, line := range ref.coverage[test].OwnedLines() {
			if plan.targets[line] {
				set[line] = true
			}
		}
		lines[test] = set
	}
	return lines
}

// allocationSelector is the selector of a scan: it allocates the operator
// mutants over the lines of the sampled tests (scanscope.Allocate).
type allocationSelector struct {
	sources map[string][]byte
	lines   map[ecosystem.TestID]scanscope.LineSet
	budget  int
	seed    uint64
}

// Select keeps the screened mutants the allocation draws, in the order it drew
// them.
func (a allocationSelector) Select(screened []operator.Mutant) []operator.Mutant {
	candidates := make([]scanscope.Candidate, 0, len(screened))
	for _, mutant := range screened {
		candidates = append(candidates, scanscope.Candidate{
			ID: mutant.ID, Family: string(mutant.Family),
			Lines: scanscope.SpanLines(mutant.Site.File, a.sources[mutant.Site.File], mutant.Site.Span),
		})
	}
	allocation := scanscope.Allocate(candidates, a.lines, a.budget, a.seed)
	byID := make(map[string]operator.Mutant, len(screened))
	for _, mutant := range screened {
		byID[mutant.ID] = mutant
	}
	selected := make([]operator.Mutant, 0, len(allocation.Order))
	for _, id := range allocation.Order {
		selected = append(selected, byID[id])
	}
	return selected
}

// mutableFiles reads the files of the targets a scan may mutate and a mutation
// engine handles.
func (s *Service) mutableFiles(layout ecosystem.Layout, paths []string) ([]mutation.SourceFile, error) {
	var files []mutation.SourceFile
	for _, path := range paths {
		if _, err := s.deps.Engines.For(path); err != nil {
			continue
		}
		content, err := fs.ReadFile(s.deps.Project, path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		if scanscope.Mutable(path, content, layout) {
			files = append(files, mutation.SourceFile{Path: path, Content: content})
		}
	}
	return files, nil
}

// admits keeps a site whose lines include a target and none of which calls a
// sink.
func admits(targets scanscope.LineSet, file mutation.SourceFile, site operator.Site) bool {
	lines := scanscope.SpanLines(file.Path, file.Content, site.Span)
	if !targets.Intersects(lines) {
		return false
	}
	return !slices.ContainsFunc(lines, func(line vacuity.Line) bool {
		return scanscope.SinkLine(scanscope.LineText(file.Content, line.Line))
	})
}

// candidateOf reads a generated mutant as a candidate of the scan.
func candidateOf(mutant operator.Mutant, source []byte) (Candidate, error) {
	id, err := format.ScanEditsDigest(mutant.EditSet.ScanEdits())
	if err != nil {
		return Candidate{}, fmt.Errorf("digest a mutant: %w", err)
	}
	return Candidate{
		ID:       id,
		Edits:    mutant.EditSet,
		Origin:   format.ScanOriginOperator,
		Family:   string(mutant.Family),
		Sentinel: mutant.IsSentinel(),
		Lines:    scanscope.SpanLines(mutant.Site.File, source, mutant.Site.Span),
	}, nil
}
