package scan

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
)

// ErrNoWitnessHost is returned when a witness must be laid out in an
// ecosystem that has no witness host.
var ErrNoWitnessHost = errors.New("the ecosystem lays out no witness")

// Killer is the executed counter-evidence one route of a finding offers, with
// no model call: a test that fails on the mutant.
type Killer struct {
	Route format.ScanRoute
	// Command runs the killing test from the project root once the mutant is
	// applied; empty for an f3_difference route, which `scan replay` re-runs.
	Command []string
	// WitnessFile is, for a witness_failure route, the test file the failing
	// witness runs in.
	WitnessFile *WitnessFile
}

// WitnessFile is witnesses laid out as one test file of the project.
type WitnessFile struct {
	Path    string
	Content []byte
}

// killersOf lists the killer of each route of a mutant, in route order.
func (s *Service) killersOf(ctx context.Context, mutant format.ScanMutant, promises *promiseOutcome) ([]Killer, error) {
	var killers []Killer
	for _, route := range mutant.Routes {
		killer := Killer{Route: route}
		switch route.Kind {
		case format.ScanRouteKilledByTest:
			killer.Command = s.deps.Suite.Repro(ecosystem.TestID(route.Test))
		case format.ScanRouteWitness:
			if promises == nil {
				continue
			}
			laid, command, err := s.layOutWitness(ctx, promises.spec, promises.files[route.Witness], route.Witness)
			if err != nil {
				return nil, err
			}
			killer.WitnessFile, killer.Command = &laid, command
		}
		killers = append(killers, killer)
	}
	return killers, nil
}

// layOutWitness lays out the witnesses of one file, written against the
// promises, as a test file of the project, and gives the command that runs the
// one witness. The witness host is briefed first, since an ecosystem may lay
// witnesses out where the brief places them.
func (s *Service) layOutWitness(ctx context.Context, spec []format.ScanPromise, file []format.ScanWitness, id string) (WitnessFile, []string, error) {
	set, err := s.briefedSet(ctx, spec, file)
	if err != nil {
		return WitnessFile{}, nil, err
	}
	laid, err := s.deps.WitnessHost.Layout(s.deps.Project, set)
	if err != nil {
		return WitnessFile{}, nil, fmt.Errorf("lay out witness %s: %w", id, err)
	}
	test, known := laid.Tests[id]
	if !known {
		return WitnessFile{}, nil, fmt.Errorf("lay out witness %s: the layout has no test for it", id)
	}
	return WitnessFile{Path: laid.Path, Content: laid.Content}, s.deps.Suite.Repro(witnessTestID(test)), nil
}

// briefedSet briefs the witness host with the promises the witnesses were
// written against and rebuilds the set of one witness file from the
// statement's witnesses.
func (s *Service) briefedSet(ctx context.Context, spec []format.ScanPromise, file []format.ScanWitness) (witness.Set, error) {
	if s.deps.WitnessHost == nil {
		return witness.Set{}, ErrNoWitnessHost
	}
	if len(file) == 0 {
		return witness.Set{}, fmt.Errorf("%w: no witness to lay out", ErrUnreadableFinding)
	}
	text, _ := witnessSpec(spec)
	code, err := s.codeSources(ctx)
	if err != nil {
		return witness.Set{}, err
	}
	if _, err := s.deps.WitnessHost.Brief(s.deps.Project, code, text); err != nil {
		return witness.Set{}, fmt.Errorf("brief the witness host: %w", err)
	}
	return setOf(file), nil
}

// setOf rebuilds a witness set from the statement's witnesses of one file:
// their promise, their imports and each witness under its own id.
func setOf(file []format.ScanWitness) witness.Set {
	ordered := slices.Clone(file)
	slices.SortFunc(ordered, func(a, b format.ScanWitness) int { return strings.Compare(a.ID, b.ID) })
	set := witness.Set{Criterion: promiseRef(ordered[0].Promise), Imports: ordered[0].Imports}
	for _, one := range ordered {
		set.Witnesses = append(set.Witnesses, witness.Witness{ID: one.ID, Criterion: set.Criterion, Name: one.Name, Code: one.Code})
	}
	return set
}

// promiseReport completes the promise report with the broken promises, ranked
// in the statement's order, each with its change and its killing witness.
func (s *Service) promiseReport(ctx context.Context, out *promiseOutcome, predicate format.Scan, drawn drawn) (*PromiseReport, error) {
	if out == nil {
		return nil, nil
	}
	report := out.report
	mutants := map[string]format.ScanMutant{}
	for _, mutant := range predicate.Mutants {
		mutants[mutant.ID] = mutant
	}
	witnesses := map[string]format.ScanWitness{}
	for _, one := range predicate.Witnesses {
		witnesses[one.ID] = one
	}
	promises := map[string]format.ScanPromise{}
	for _, one := range predicate.Promises {
		promises[one.ID] = one
	}
	for _, broken := range predicate.BrokenPromises {
		mutant := mutants[broken.Mutant]
		files, err := s.fileChanges(mutant, drawn)
		if err != nil {
			return nil, err
		}
		laid, command, err := s.layOutWitness(ctx, out.spec, out.files[broken.Witness], broken.Witness)
		if err != nil {
			return nil, err
		}
		route := format.ScanRoute{Kind: format.ScanRouteWitness, Witness: broken.Witness}
		report.Broken = append(report.Broken, BrokenPromise{
			Rank: len(report.Broken) + 1, Promise: promises[broken.Promise], Mutant: mutant, Witness: witnesses[broken.Witness],
			File: out.files[broken.Witness], Site: firstLine(mutant), Files: files, Spec: out.spec,
			Killer: Killer{Route: route, Command: command, WitnessFile: &laid},
		})
	}
	return &report, nil
}

// firstLine is the site of a mutant: its first mutated line.
func firstLine(mutant format.ScanMutant) format.ScanLine {
	if len(mutant.Lines) == 0 {
		return format.ScanLine{}
	}
	return slices.MinFunc(mutant.Lines, format.CompareScanLines)
}

// emptyEdits is the program of the reference: no edit.
var emptyEdits = mutation.EditSet{}
