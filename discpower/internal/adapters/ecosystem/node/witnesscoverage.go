package node

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/coverage"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
)

var _ ports.WitnessCovering = (*Strategy)(nil)

// WitnessCoverage records, for each test it is given, the definitions of the
// project the test runs, running each test alone under the scan's coverage.
func (s *Strategy) WitnessCoverage() ports.CoverageRecorder {
	return witnessCoverage{suite: scanSuite{deps: s.deps, framework: s.framework, cache: s.scanCache}, locator: s.deps.Locator}
}

// witnessCoverage is the Adapter of the scan's recordings to the units a
// coverage map names.
type witnessCoverage struct {
	suite   scanSuite
	locator ports.DefinitionLocator
}

// Record runs each test alone once, within the strategy's timeout, and names
// the definitions holding a line it ran inside its window, the lines of test
// files aside. A test that fails still runs what it ran; a run past the
// timeout fails the recording.
func (c witnessCoverage) Record(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, tests []ecosystem.BoundTest) (coverage.Map, error) {
	recorder := c.suite.recorder(workspace)
	definitions := map[string][]ecosystem.Definition{}
	recorded := coverage.Map{}
	for _, test := range tests {
		id := scanIDOfBound(test)
		ran, err := c.suite.runProcess(ctx, workspace, toolchain, scanProcess{tests: []ecosystem.TestID{id}, coverage: true, timeout: c.suite.deps.Timeout})
		if err != nil {
			return nil, err
		}
		if ran.timedOut {
			return nil, fmt.Errorf("%w: %s ran past %s", ports.ErrCoverageTimedOut, id, c.suite.deps.Timeout)
		}
		lines := map[vacuity.Line]bool{}
		for _, process := range ran.record.processes {
			for line, units := range analyzeProcess(process, recorder.shapes).counts {
				if recorder.source(line) && ranInWindow(units) {
					lines[line] = true
				}
			}
		}
		recorded[test.ID] = c.unitsOf(workspace, lines, definitions)
	}
	return recorded, nil
}

// ranInWindow reports whether a code unit of a line counted in the window.
func ranInWindow(units map[unitKey]unitCounts) bool {
	for _, unit := range units {
		if unit.window > 0 {
			return true
		}
	}
	return false
}

// unitsOf names the innermost definition holding each line, sorted by file then
// qualified name; a file the locator cannot read names none.
func (c witnessCoverage) unitsOf(workspace ecosystem.Workspace, lines map[vacuity.Line]bool, definitions map[string][]ecosystem.Definition) []footprint.Ref {
	seen := map[footprint.Ref]bool{}
	refs := []footprint.Ref{}
	for line := range lines {
		located, read := definitions[line.File]
		if !read {
			located = c.locate(workspace, line.File)
			definitions[line.File] = located
		}
		var best ecosystem.Definition
		found := false
		for _, definition := range located {
			if line.Line >= definition.StartLine && line.Line <= definition.EndLine && (!found || definition.EndLine-definition.StartLine < best.EndLine-best.StartLine) {
				best, found = definition, true
			}
		}
		if ref := (footprint.Ref{File: line.File, Qualname: best.Qualname}); found && !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	slices.SortFunc(refs, func(a, b footprint.Ref) int {
		if byFile := strings.Compare(a.File, b.File); byFile != 0 {
			return byFile
		}
		return strings.Compare(a.Qualname, b.Qualname)
	})
	return refs
}

func (c witnessCoverage) locate(workspace ecosystem.Workspace, file string) []ecosystem.Definition {
	if c.locator == nil {
		return nil
	}
	content, err := os.ReadFile(filepath.Join(workspace.Dir, filepath.FromSlash(file)))
	if err != nil {
		return nil
	}
	located, err := c.locator.Locate(file, content)
	if err != nil {
		return nil
	}
	return located
}

// scanIDOfBound is the scan's id of a test laid out in the witness suite: the
// file, the suite's title and the test's, whichever separator the runner's
// report joins them with.
func scanIDOfBound(test ecosystem.BoundTest) ecosystem.TestID {
	title := test.Name
	if rest, found := strings.CutPrefix(title, witnessSuite); found {
		rest = strings.TrimPrefix(strings.TrimPrefix(rest, vitestTitleSeparator), jestTitleSeparator)
		title = witnessSuite + titleSeparator + rest
	}
	return ecosystem.TestID(test.File + testIDSeparator + title)
}
