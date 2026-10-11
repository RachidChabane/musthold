package scan

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
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
}

// draw generates the operator mutants and sentinels of the files the targets
// lie in, keeps those whose mutated lines are targets and call no sink, and
// samples the operator mutants by site and family.
func (s *Service) draw(ref reference, plan plan, request Request) (drawn, error) {
	files, err := s.mutableFiles(ref.layout, plan.targets.Files())
	if err != nil {
		return drawn{}, err
	}
	sources := map[string][]byte{}
	for _, file := range files {
		sources[file.Path] = file.Content
	}
	generated, err := operatorgen.NewGenerator(s.deps.Engines).Generate(operatorgen.Request{
		Files:  files,
		Config: operator.Config{KOp: request.Mutants, Seed: scanscope.DrawSeed(request.Seed)},
		Admit: func(file mutation.SourceFile, site operator.Site) bool {
			return admits(plan.targets, file, site)
		},
	})
	if err != nil {
		return drawn{}, fmt.Errorf("draw the mutants: %w", err)
	}
	result := drawn{sources: sources}
	seen := map[string]bool{}
	for _, mutant := range slices.Concat(generated.Operator, generated.Sentinels) {
		candidate, err := candidateOf(mutant, sources[mutant.Site.File])
		if err != nil {
			return drawn{}, err
		}
		if !seen[candidate.ID] && len(candidate.Lines) > 0 {
			seen[candidate.ID] = true
			result.candidates = append(result.candidates, candidate)
		}
	}
	slices.SortFunc(result.candidates, func(a, b Candidate) int { return strings.Compare(a.ID, b.ID) })
	return result, nil
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
