// Package ecosystem holds the value types shared by every test ecosystem
// (pytest, Jest, Vitest, Maven, Gradle, Go testing): names, detections, source
// definitions with byte spans, test identities and outcomes.
package ecosystem

import (
	"errors"
	"fmt"
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

// DefinitionKind tells a free function from a method.
type DefinitionKind string

// Definition kinds.
const (
	KindFunction DefinitionKind = "function"
	KindMethod   DefinitionKind = "method"
)

// Definition is a function or method definition of a source file. Qualname uses
// the ecosystem's own grammar: `Class.method`, `pkg.Func`, `(*T).Method` or
// `Outer.inner`.
type Definition struct {
	Qualname string
	Kind     DefinitionKind
	Span     Span
}

// TestID is the identity of a test as the ecosystem's JUnit reporter names it.
type TestID string

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
	ID   TestID
	File string
	Line int
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
