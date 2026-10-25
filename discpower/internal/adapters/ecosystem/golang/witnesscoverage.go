package golang

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/coverage"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
)

var _ ports.WitnessCovering = (*Strategy)(nil)

// WitnessCoverage records, for each test it is given, the definitions of the
// module the test runs, running each test alone from its package's coverage
// binary.
func (s *Strategy) WitnessCoverage() ports.CoverageRecorder {
	return witnessCoverage{suite: scanSuite{deps: s.deps}, locator: s.deps.Locator}
}

// witnessCoverage is the coverage recorder of Go tests run alone: the Adapter
// of the scan's coverage binaries to the units a coverage map names.
type witnessCoverage struct {
	suite   scanSuite
	locator ports.DefinitionLocator
}

// Record builds the coverage binary of each package that holds a test, runs
// its start-up once with no test, then each test alone, within the strategy's
// timeout, and names the definitions holding a block the test ran more often
// than the start-up did. A test whose run leaves no profile runs nothing; a
// test that fails still runs what it ran.
func (c witnessCoverage) Record(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, tests []ecosystem.BoundTest) (coverage.Map, error) {
	module, err := modulePath(os.DirFS(workspace.Dir))
	if err != nil {
		return nil, err
	}
	units := newUnitIndex(workspace.Dir, c.locator)
	recorded := coverage.Map{}
	for _, group := range testsByPackage(tests) {
		built, err := c.suite.buildCoverageBinary(ctx, workspace, toolchain, group.dir)
		if err != nil {
			return nil, err
		}
		binary := &coverageBinary{path: built, dir: group.dir}
		startUp, err := c.counts(ctx, workspace, binary, startUpOnly)
		if err != nil {
			return nil, err
		}
		for _, test := range group.tests {
			ran, err := c.counts(ctx, workspace, binary, expression([][]string{strings.Split(test.Name, subtestSeparator)}))
			if err != nil {
				return nil, err
			}
			recorded[test.ID] = units.of(ranBeyond(ran, startUp), module)
		}
	}
	return recorded, nil
}

// counts runs the tests of a binary the expression selects once and reads the
// blocks the run counted; a run that left no profile counted none.
func (c witnessCoverage) counts(ctx context.Context, workspace ecosystem.Workspace, binary *coverageBinary, selection string) (blockCounts, error) {
	ran, err := c.suite.runBinary(ctx, workspace, binary, selection, c.suite.deps.Timeout)
	if err != nil {
		return nil, err
	}
	if ran.timedOut {
		return nil, fmt.Errorf("%w: %s of %s ran past %s", ports.ErrCoverageTimedOut, selection, binary.dir, c.suite.deps.Timeout)
	}
	if ran.profile == nil {
		return blockCounts{}, nil
	}
	return readBlockCounts(ran.profile), nil
}

// ranBeyond keeps the blocks a test ran more often than the start-up of its
// binary did.
func ranBeyond(ran, startUp blockCounts) []profileBlock {
	var blocks []profileBlock
	for position, entry := range ran {
		if entry.count > startUp[position].count {
			blocks = append(blocks, entry.block)
		}
	}
	return blocks
}

// packageTests are the tests of one package directory, in the order given.
type packageTests struct {
	dir   string
	tests []ecosystem.BoundTest
}

// testsByPackage groups the tests by the directory of their file, in the order
// each directory first appears.
func testsByPackage(tests []ecosystem.BoundTest) []packageTests {
	var groups []packageTests
	for _, test := range tests {
		dir := path.Dir(test.File)
		index := slices.IndexFunc(groups, func(group packageTests) bool { return group.dir == dir })
		if index < 0 {
			groups = append(groups, packageTests{dir: dir})
			index = len(groups) - 1
		}
		groups[index].tests = append(groups[index].tests, test)
	}
	return groups
}

// unitIndex names the innermost definition holding a line of a file of the
// module, reading and locating each file once.
type unitIndex struct {
	root        string
	locator     ports.DefinitionLocator
	definitions map[string][]ecosystem.Definition
}

func newUnitIndex(root string, locator ports.DefinitionLocator) *unitIndex {
	return &unitIndex{root: root, locator: locator, definitions: map[string][]ecosystem.Definition{}}
}

// of names the definitions holding a line of the blocks, sorted by file then
// qualified name; a block of a file outside the module, of a test file or of
// a file the locator cannot read names none.
func (u *unitIndex) of(blocks []profileBlock, module string) []footprint.Ref {
	seen := map[footprint.Ref]bool{}
	refs := []footprint.Ref{}
	for _, block := range blocks {
		file, inside := block.relativeFile(module)
		if !inside || strings.HasSuffix(file, witnessFileSuffix) {
			continue
		}
		definitions := u.definitionsOf(file)
		for line := block.first; line <= block.last; line++ {
			definition, found := innermost(definitions, line)
			ref := footprint.Ref{File: file, Qualname: definition.Qualname}
			if found && !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
	}
	slices.SortFunc(refs, func(a, b footprint.Ref) int {
		return strings.Compare(a.File+unitSeparator+a.Qualname, b.File+unitSeparator+b.Qualname)
	})
	return refs
}

// unitSeparator orders units by file, then qualified name.
const unitSeparator = "\x00"

// definitionsOf locates the definitions of a file of the module once; a file
// that cannot be read or located has none.
func (u *unitIndex) definitionsOf(file string) []ecosystem.Definition {
	if definitions, read := u.definitions[file]; read {
		return definitions
	}
	var definitions []ecosystem.Definition
	if content, err := os.ReadFile(filepath.Join(u.root, filepath.FromSlash(file))); err == nil && u.locator != nil {
		if located, err := u.locator.Locate(file, content); err == nil {
			definitions = located
		}
	}
	u.definitions[file] = definitions
	return definitions
}

// innermost is the definition of the fewest lines that holds the line.
func innermost(definitions []ecosystem.Definition, line int) (ecosystem.Definition, bool) {
	var best ecosystem.Definition
	found := false
	for _, definition := range definitions {
		if line < definition.StartLine || line > definition.EndLine {
			continue
		}
		if !found || definition.EndLine-definition.StartLine < best.EndLine-best.StartLine {
			best, found = definition, true
		}
	}
	return best, found
}
