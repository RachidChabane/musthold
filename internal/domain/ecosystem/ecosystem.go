// Package ecosystem holds the value types shared by every test ecosystem
// (pytest, Jest, Vitest, Maven, Gradle, Go testing): names, detections, source
// definitions with byte spans, test identities and outcomes.
package ecosystem

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"slices"
)

// Name identifies a supported test ecosystem.
type Name string

// Supported ecosystems, one per runner Shallnot binds.
const (
	Pytest    Name = "pytest"
	Jest      Name = "jest"
	Vitest    Name = "vitest"
	Maven     Name = "maven"
	Gradle    Name = "gradle"
	GoTesting Name = "go"
)

var allNames = []Name{Pytest, Jest, Vitest, Maven, Gradle, GoTesting}

// Names lists every supported ecosystem in a stable order.
func Names() []Name { return slices.Clone(allNames) }

// Valid reports whether n is a supported ecosystem.
func (n Name) Valid() bool { return slices.Contains(allNames, n) }

// Detection is the result of recognizing an ecosystem in a project tree.
type Detection struct {
	Name Name
	// Root is the project directory, relative POSIX, "." for the tree root.
	Root string
	// Evidence lists the files that revealed the ecosystem.
	Evidence []string
}

// Workspace is a directory a command runs in, outside the repository.
type Workspace struct {
	Dir string
	// ResultsDir holds the runner's result files of this workspace. It lies
	// outside Dir, so result files are never cloned with the tree.
	ResultsDir string
}

// Toolchain is the resolved command prefix of an ecosystem's tools, for example
// ["uv", "run", "python"] or ["./mvnw"].
type Toolchain struct {
	Command []string
}

// ErrInvalidSpan is returned for a span whose bounds are negative or inverted.
var ErrInvalidSpan = errors.New("invalid span")

// Span is a half-open byte range [Start, End) of a source file.
type Span struct {
	Start int
	End   int
}

// NewSpan builds a span, rejecting negative or inverted bounds.
func NewSpan(start, end int) (Span, error) {
	if start < 0 || end < start {
		return Span{}, fmt.Errorf("%w: [%d,%d)", ErrInvalidSpan, start, end)
	}
	return Span{Start: start, End: end}, nil
}

// Len returns the number of bytes the span covers.
func (s Span) Len() int { return s.End - s.Start }

// Within reports whether the span lies inside a source of the given length.
func (s Span) Within(length int) bool { return s.Start >= 0 && s.End >= s.Start && s.End <= length }

// Slice returns the bytes of source the span covers.
func (s Span) Slice(source []byte) ([]byte, error) {
	if !s.Within(len(source)) {
		return nil, fmt.Errorf("%w: [%d,%d) outside a source of %d bytes", ErrInvalidSpan, s.Start, s.End, len(source))
	}
	return source[s.Start:s.End], nil
}

// Lines returns the 1-based first and last line the span covers in source. The
// span must lie within source.
func (s Span) Lines(source []byte) (first, last int) {
	first = 1 + bytes.Count(source[:s.Start], []byte("\n"))
	return first, first + bytes.Count(source[s.Start:s.End], []byte("\n"))
}

// NewAssignment is the single definition of an assignment unit of a source: its
// qualname, kind, span and line range.
func NewAssignment(source []byte, qualname string, span Span) Definition {
	first, last := span.Lines(source)
	return Definition{Qualname: qualname, Kind: KindAssignment, Span: span, StartLine: first, EndLine: last}
}

// DefinitionKind tells a free function from a method, or marks a module-level
// assignment unit.
type DefinitionKind string

// Definition kinds.
const (
	KindFunction DefinitionKind = "function"
	KindMethod   DefinitionKind = "method"
	// KindAssignment is a module-level assignment (Python) or a package-level
	// const or var spec (Go), located by its assigned name. It is returned only
	// by an assignment locator, never by a definition locator.
	KindAssignment DefinitionKind = "assignment"
)

// Definition is a function or method definition of a source file. Qualname uses
// the ecosystem's own grammar: `Class.method`, `pkg.Func`, `(*T).Method` or
// `Outer.inner`.
type Definition struct {
	Qualname string
	Kind     DefinitionKind
	Span     Span
	// StartLine and EndLine are the 1-based first and last lines of the span.
	StartLine int
	EndLine   int
}

// TestID is the identity of a test as the ecosystem's JUnit reporter names it.
type TestID string

// testIDSeparator joins the classname and the name of a test case.
const testIDSeparator = "::"

// NewTestID is the single definition of a test identity: the classname and the
// name the JUnit reporter gives a test case, joined. Bound tests read from a
// report and outcomes read from a results file both use it.
func NewTestID(classname, name string) TestID { return TestID(classname + testIDSeparator + name) }

// TestOutcome is the outcome of one test in one suite run.
type TestOutcome string

// Test outcomes read from JUnit XML. TestFailed covers the failure and error
// elements raised by a test body; TestErrored means the test could not run
// (setup or collection problem), which proves nothing about the code.
const (
	TestPassed  TestOutcome = "passed"
	TestFailed  TestOutcome = "failed"
	TestErrored TestOutcome = "errored"
	TestSkipped TestOutcome = "skipped"
)

// BoundTest is a test bound to a criterion by Shallnot's report: its identity and
// the source location it is declared at.
type BoundTest struct {
	ID        TestID
	Classname string
	Name      string
	File      string
	Line      int
}

// NewBoundTest builds a bound test from the names the report gives it.
func NewBoundTest(classname, name, file string, line int) BoundTest {
	return BoundTest{ID: NewTestID(classname, name), Classname: classname, Name: name, File: file, Line: line}
}

// Selection is the native selection of an ecosystem's runner for a set of bound
// tests: the runner arguments and the tests they are meant to select.
type Selection struct {
	Args  []string
	Tests []TestID
}

// BuildResult is the outcome of a compile or import smoke check.
type BuildResult struct {
	OK          bool
	Diagnostics string
}

// RunResult is what a suite run reports.
type RunResult struct {
	Outcomes         map[TestID]TestOutcome
	TimedOut         bool
	CollectionFailed bool
}

// outcomeSeverity orders outcomes when one test identity is reported several
// times; the highest wins.
var outcomeSeverity = map[TestOutcome]int{TestSkipped: 0, TestPassed: 1, TestErrored: 2, TestFailed: 3}

// MergeOutcome returns the outcome that stands for two reports of one test: a
// failure outranks an error, an error outranks a pass, a pass outranks a skip.
func MergeOutcome(a, b TestOutcome) TestOutcome {
	if outcomeSeverity[b] > outcomeSeverity[a] {
		return b
	}
	return a
}

// ReportFormat names the JUnit XML dialect of a runner's reporter.
type ReportFormat string

// Report formats, one per reporter of the supported ecosystems.
const (
	ReportPytest    ReportFormat = "pytest"
	ReportJestJUnit ReportFormat = "jest-junit"
	ReportVitest    ReportFormat = "vitest"
	ReportSurefire  ReportFormat = "surefire"
	ReportGradle    ReportFormat = "gradle"
	ReportGotestsum ReportFormat = "gotestsum"
)

// Layout is what the workspace and the splicing policy need to know about the
// files of an ecosystem.
type Layout struct {
	// ExcludedDirs are directory names never copied into a workspace.
	ExcludedDirs []string
	// DependencyDirs are directory names (relative to the project root) that
	// hold installed dependencies; they are cloned or symlinked, and ignored
	// when the modification-time rule looks for build outputs.
	DependencyDirs []string
	// TestFileGlobs are base-name patterns of test files, matched with path.Match.
	TestFileGlobs []string
}

// IsTestFile reports whether the file at filePath is a test file of the layout.
func (l Layout) IsTestFile(filePath string) bool {
	base := path.Base(filePath)
	for _, pattern := range l.TestFileGlobs {
		if matched, err := path.Match(pattern, base); err == nil && matched {
			return true
		}
	}
	return false
}

// EnclosingDefinition returns the definition a source line belongs to: the
// innermost one containing the line, else the nearest one starting after it
// (the line then holds a decorator or an annotation of that definition).
func EnclosingDefinition(definitions []Definition, line int) (Definition, bool) {
	var best Definition
	found := false
	for _, d := range definitions {
		if d.StartLine <= line && line <= d.EndLine && (!found || d.Span.Len() < best.Span.Len()) {
			best, found = d, true
		}
	}
	if found {
		return best, true
	}
	for _, d := range definitions {
		if d.StartLine > line && (!found || d.StartLine < best.StartLine) {
			best, found = d, true
		}
	}
	return best, found
}
