package prove

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/execution"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
)

// staticAnalysis is what the deterministic stages learn about one criterion
// before any host answer is needed: its footprint, the operator mutants and
// sentinels over it, the slice of code a generation request shows, and the
// inputs of the differential search.
type staticAnalysis struct {
	criterion   spec.Criterion
	generated   operatorgen.Result
	slice       []prompt.SliceEntry
	codeFiles   []string
	testFiles   []string
	draws       *execution.Draws
	skipReason  string
	sourceTexts []string
}

// staticOf computes, once per run and footprint, the static analysis of a
// criterion from the entry functions of its footprint.
func (p *project) staticOf(ctx context.Context, criterion spec.Criterion, entries []footprint.Ref) (*staticAnalysis, error) {
	key := criterion.Ref.String() + seedSeparator + footprintKey(entries)
	if cached, done := p.statics[key]; done {
		return cached, nil
	}
	generated, err := operatorgen.NewGenerator(p.deps.Engines).Generate(operatorgen.Request{
		Files:   p.sources,
		Entries: entries,
		Config:  operator.Config{KOp: p.settings.Policy.KOp, Seed: operator.Seed(p.settings.Seed)},
	})
	if err != nil {
		return nil, fmt.Errorf("generate the operator mutants of %s: %w", criterion.Ref, err)
	}
	analysis := &staticAnalysis{criterion: criterion, generated: generated}
	if analysis.slice, err = p.sliceOf(generated.Footprint); err != nil {
		return nil, err
	}
	analysis.codeFiles = p.codeFilesOf(generated.Footprint)
	analysis.testFiles = p.testFilesOf(criterion, generated.Footprint)
	for _, entry := range analysis.slice {
		analysis.sourceTexts = append(analysis.sourceTexts, entry.Source)
	}
	analysis.draws, analysis.skipReason, err = p.runner.PrepareDraws(ctx, analysis.sourceTexts, p.settings.Seed)
	if err != nil {
		return nil, err
	}
	if analysis.draws == nil {
		p.findings = append(p.findings, fmt.Sprintf("%s: %s %s", FindingDifferentialSkipped, criterion.Ref, analysis.skipReason))
	}
	p.statics[key] = analysis
	p.footprints[criterion.Ref] = entries
	return analysis, nil
}

// footprintKey names the entry functions of a footprint.
func footprintKey(entries []footprint.Ref) string {
	parts := make([]string, len(entries))
	for index, entry := range entries {
		parts[index] = entry.File + "\x00" + entry.Qualname
	}
	return digestText(strings.Join(parts, "\x01"))
}

// entriesOf returns the entry functions of a criterion's footprint: the units its
// witnesses execute on the reference, so that the code a generation request
// shows never depends on the tests. An ecosystem that cannot run witnesses takes
// the units the criterion's tests execute. No entry puts every unit of the code
// in scope.
//
// Only the first draw's witnesses count: a further draw comes after the search
// ended, and the footprint the search ran on stays the one its requests showed.
func (r *critRun) entriesOf(ctx context.Context) ([]footprint.Ref, error) {
	if !r.p.runner.Witnesses.Available() {
		return r.p.runner.Coverage.EntriesOf(r.c.TestIDs()), nil
	}
	var first []witness.Set
	for _, draw := range r.draws {
		if draw.number == firstDraw {
			first = append(first, r.runnableSetsOf(draw)...)
		}
	}
	return r.p.runner.WitnessEntries(ctx, first)
}

// codeFilesOf lists the files the criterion's code tree consists of: the files of
// the footprint, or every source file when no footprint restricts the scope.
func (p *project) codeFilesOf(closed footprint.Footprint) []string {
	if files := closed.Files(); len(files) > 0 {
		return files
	}
	files := make([]string, len(p.sources))
	for index, source := range p.sources {
		files[index] = source.Path
	}
	return files
}

// testFilesOf lists the test files the criterion's proof depends on: those that
// hold a test bound to it, and those that hold a test whose execution reaches the
// footprint under the reference coverage. Without a footprint, every file that
// holds a bound test of the project is reached.
func (p *project) testFilesOf(criterion spec.Criterion, closed footprint.Footprint) []string {
	files := map[string]bool{}
	for _, test := range criterion.Tests {
		files[test.File] = true
	}
	for _, test := range p.reachingTests(closed) {
		files[test.File] = true
	}
	return slices.Sorted(maps.Keys(files))
}

// reachingTests lists the tests of the project that execute a definition of the
// footprint, or all of them when no footprint restricts the scope.
func (p *project) reachingTests(closed footprint.Footprint) []ecosystem.BoundTest {
	if len(closed.Definitions) == 0 {
		return p.tests
	}
	ids := make([]ecosystem.TestID, len(p.tests))
	for index, test := range p.tests {
		ids[index] = test.ID
	}
	covering := p.runner.Coverage.Covering(ids, closed.Definitions)
	return slices.DeleteFunc(slices.Clone(p.tests), func(test ecosystem.BoundTest) bool { return !slices.Contains(covering, test.ID) })
}

// sliceOf shows the units of the footprint, or of every source file when no
// footprint restricts the scope.
func (p *project) sliceOf(closed footprint.Footprint) ([]prompt.SliceEntry, error) {
	if len(closed.Definitions)+len(closed.Assignments) == 0 {
		return p.sliceOfEverything()
	}
	slice, err := p.appendUnits(nil, closed.Definitions, mutation.TargetDefinition)
	if err != nil {
		return nil, err
	}
	return p.appendUnits(slice, closed.Assignments, mutation.TargetModuleAssignment)
}

func (p *project) appendUnits(slice []prompt.SliceEntry, refs []footprint.Ref, target mutation.TargetKind) ([]prompt.SliceEntry, error) {
	for _, ref := range refs {
		source, err := p.unitSource(ref.File, ref.Qualname, target)
		if err != nil {
			return nil, err
		}
		slice = append(slice, prompt.SliceEntry{File: ref.File, Qualname: ref.Qualname, Source: source})
	}
	return slice, nil
}

func (p *project) sliceOfEverything() ([]prompt.SliceEntry, error) {
	var slice []prompt.SliceEntry
	for _, source := range p.sources {
		units, err := p.unitsOf(source)
		if err != nil {
			return nil, err
		}
		for _, unit := range units {
			text, err := unit.Span.Slice(source.Content)
			if err != nil {
				return nil, err
			}
			slice = append(slice, prompt.SliceEntry{File: source.Path, Qualname: unit.Qualname, Source: string(text)})
		}
	}
	return slice, nil
}

// unitsOf lists the definitions and the module-level assignments of a source.
func (p *project) unitsOf(source mutation.SourceFile) ([]ecosystem.Definition, error) {
	units, err := p.deps.Strategy.Locator().Locate(source.Path, source.Content)
	if err != nil {
		return nil, fmt.Errorf("locate the definitions of %q: %w", source.Path, err)
	}
	if assignments := p.deps.Strategy.Assignments(); assignments != nil {
		more, err := assignments.LocateAssignments(source.Path, source.Content)
		if err != nil {
			return nil, fmt.Errorf("locate the assignments of %q: %w", source.Path, err)
		}
		units = append(units, more...)
	}
	return units, nil
}

// unitSource returns the source text of a unit of the reference.
func (p *project) unitSource(file, qualname string, target mutation.TargetKind) (string, error) {
	for _, source := range p.sources {
		if source.Path != file {
			continue
		}
		units, err := p.unitsOf(source)
		if err != nil {
			return "", err
		}
		wantKind := ecosystem.KindFunction
		if target == mutation.TargetModuleAssignment {
			wantKind = ecosystem.KindAssignment
		}
		for _, unit := range units {
			if unit.Qualname == qualname && (unit.Kind == wantKind || (wantKind == ecosystem.KindFunction && unit.Kind == ecosystem.KindMethod)) {
				text, err := unit.Span.Slice(source.Content)
				return string(text), err
			}
		}
	}
	return "", fmt.Errorf("no %s %q in %q", target, qualname, file)
}
