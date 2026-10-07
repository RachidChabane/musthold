package prove

import (
	"context"
	"fmt"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/execution"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
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
	draws       *execution.Draws
	skipReason  string
	sourceTexts []string
}

// staticOf computes, once per run, the static analysis of a criterion.
func (p *project) staticOf(ctx context.Context, criterion spec.Criterion) (*staticAnalysis, error) {
	if cached, done := p.statics[criterion.Ref]; done {
		return cached, nil
	}
	entries := p.runner.Coverage.EntriesOf(criterion.TestIDs())
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
	p.statics[criterion.Ref] = analysis
	return analysis, nil
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
