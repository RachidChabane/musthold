// Package justify decides which changed units a requirement justifies: a unit is
// justified when executing an example probe of a requirement, or an ALIGNED test
// of a requirement, may run it (ADR 0003, "Completeness"). Execution is read
// from static reachability over the units of the project, because completeness
// runs no code; the evidence that a probe or a test exists comes from Statements
// of other modules.
package justify

import (
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/reach"
)

// Source says what kind of evidence a justification rests on.
type Source string

// Sources of justification.
const (
	// SourceProbe: an example of the requirement that its discriminating-power
	// Statement lists.
	SourceProbe Source = "example_probe"
	// SourceAlignedTest: a test the requirement's alignment Statements find ALIGNED.
	SourceAlignedTest Source = "aligned_test"
)

// Evidence is a set of entry names, the functions a probe or a test starts
// executing, that belong to one requirement.
type Evidence struct {
	Ref     criterion.Ref
	Source  Source
	Entries []string
}

// Justifiers maps the key of every unit that some evidence may execute to the
// requirements that justify it, sorted and without repeats.
func Justifiers(graph reach.Graph, evidence []Evidence) map[string][]criterion.Ref {
	justifiers := map[string][]criterion.Ref{}
	for _, item := range evidence {
		for key := range graph.Reached(item.Entries) {
			if !slices.Contains(justifiers[key], item.Ref) {
				justifiers[key] = append(justifiers[key], item.Ref)
			}
		}
	}
	for _, refs := range justifiers {
		slices.SortFunc(refs, criterion.Ref.Compare)
	}
	return justifiers
}

// Prefixes of the names the test runners collect: Python and unittest tests start
// with `test`, Go tests, examples and benchmarks with a capitalized word.
var testNamePrefixes = []string{"test", "Test", "Example", "Benchmark"}

// TestEntries lists the function names a test label designates. The label of an
// alignment Statement is `classname.name`; a Go subtest adds `/case`, and a
// parametrized Python test adds `[case]`. Every segment of the label that reads
// like the name of a test function is an entry, because the segments cannot be
// told from the package or class path without knowing the language.
func TestEntries(label string) []string {
	segments := strings.FieldsFunc(label, func(r rune) bool { return strings.ContainsRune("./[]: ", r) })
	var entries []string
	for _, segment := range segments {
		if slices.ContainsFunc(testNamePrefixes, func(prefix string) bool { return strings.HasPrefix(segment, prefix) }) {
			entries = append(entries, segment)
		}
	}
	return entries
}

// OperationEntry is the function name an example operation binding designates:
// `module:function`, `module:Class.method` or the bare operation name.
func OperationEntry(binding string) string {
	if index := strings.LastIndex(binding, ":"); index >= 0 {
		binding = binding[index+1:]
	}
	if index := strings.LastIndex(binding, "."); index >= 0 {
		binding = binding[index+1:]
	}
	return binding
}
