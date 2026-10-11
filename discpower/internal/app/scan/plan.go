package scan

import (
	"context"
	"fmt"
	"io/fs"
	"maps"
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

// plan is what a scan judges: the sampled tests and the lines a draw may
// mutate.
type plan struct {
	mode    format.ScanMode
	sampled []ecosystem.TestID
	// targets are the lines a mutant may mutate: the lines the sampled tests
	// ran, within the functions a change edits for a change scan.
	targets scanscope.LineSet
}

// plan samples the tests of a repository scan, or takes the tests that run the
// functions a change edits, records the coverage the scope of the request asks
// for, and the lines a draw may mutate.
func (s *Service) plan(ctx context.Context, ref *reference, request Request, base string) (plan, error) {
	if base == "" {
		sampled := scanscope.Sample(scanCasesOf(ref.cases, ref.green), request.SampleSize, request.Seed)
		if err := s.recordCoverage(ctx, ref, request.Coverage.of(ref.green, sampled, s.deps.Suite.CoverageOfOneRun()), request); err != nil {
			return plan{}, err
		}
		return plan{mode: format.ScanModeRepository, sampled: sampled, targets: s.coveredBy(*ref, sampled)}, nil
	}
	changed, err := s.changedFunctions(ctx, base, ref.layout)
	if err != nil {
		return plan{}, err
	}
	if err := s.recordCoverage(ctx, ref, ref.green, request); err != nil {
		return plan{}, err
	}
	var running []ports.ScanCase
	for _, scanCase := range ref.cases {
		if vacuity.Green(ref.runs[scanCase.ID]) && changed.Intersects(ref.coverage[scanCase.ID].Lines) {
			running = append(running, scanCase)
		}
	}
	sampled := scanscope.Sample(scanCasesOf(running, ref.green), request.SampleSize, request.Seed)
	targets := scanscope.LineSet{}
	for line := range s.coveredBy(*ref, sampled) {
		if changed[line] {
			targets[line] = true
		}
	}
	return plan{mode: format.ScanModeChange, sampled: sampled, targets: targets}, nil
}

// coveredBy is the set of lines the given tests ran, start-up included.
func (s *Service) coveredBy(ref reference, tests []ecosystem.TestID) scanscope.LineSet {
	set := scanscope.LineSet{}
	for _, test := range tests {
		for _, line := range ref.coverage[test].Lines {
			set[line] = true
		}
	}
	return set
}

// changedFunctions are the lines of the functions whose lines differ from the
// base, in the files a scan may mutate.
func (s *Service) changedFunctions(ctx context.Context, base string, layout ecosystem.Layout) (scanscope.LineSet, error) {
	changed, err := s.deps.Source.ChangedLines(ctx, base)
	if err != nil {
		return nil, fmt.Errorf("read the changed lines: %w", err)
	}
	set := scanscope.LineSet{}
	for _, file := range slices.Sorted(maps.Keys(changed)) {
		content, err := fs.ReadFile(s.deps.Project, file)
		if err != nil || !scanscope.Mutable(file, content, layout) {
			continue
		}
		if _, err := s.deps.Engines.For(file); err != nil {
			continue
		}
		definitions, err := s.deps.Ecosystem.Locator().Locate(file, content)
		if err != nil {
			continue
		}
		for _, line := range scanscope.DefinitionLines(file, definitions, changed[file]) {
			set[line] = true
		}
	}
	return set, nil
}

// scanCasesOf keeps the green cases as the sample reads them.
func scanCasesOf(cases []ports.ScanCase, green []ecosystem.TestID) []scanscope.Case {
	isGreen := make(map[ecosystem.TestID]bool, len(green))
	for _, test := range green {
		isGreen[test] = true
	}
	var kept []scanscope.Case
	for _, scanCase := range cases {
		if isGreen[scanCase.ID] {
			kept = append(kept, scanscope.Case{ID: scanCase.ID, File: scanCase.File})
		}
	}
	return kept
}
