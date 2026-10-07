package javascript

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/alignment/internal/platform/testname"
)

var errNoGrammar = errors.New("no grammar serves the file")

// memberKind tells what a statement of a block is for the masker.
type memberKind int

const (
	memberOther memberKind = iota
	// memberDeclaration binds names a test may use.
	memberDeclaration
	// memberSuite is a `describe` call.
	memberSuite
	// memberTest is an `it` or `test` call.
	memberTest
	// memberSetup is a hook or a framework setup call, which runs for every test of
	// its block.
	memberSetup
)

// member is a statement of a module or of the callback of a suite.
type member struct {
	kind      memberKind
	statement *gotreesitter.Node
	call      call
	names     []string
}

// membersOf classifies the statements of a program or of a suite's block.
func (x *fileIndex) membersOf(container *gotreesitter.Node) []member {
	var members []member
	for _, statement := range x.statementsOf(container) {
		members = append(members, x.memberOf(statement))
	}
	return members
}

func (x *fileIndex) memberOf(statement *gotreesitter.Node) member {
	read := x.classify(statement)
	switch read.kind {
	case callSuite:
		return member{kind: memberSuite, statement: statement, call: read}
	case callTest:
		return member{kind: memberTest, statement: statement, call: read}
	case callHook, callSetup:
		return member{kind: memberSetup, statement: statement, call: read}
	}
	if names := x.declaredNames(statement); len(names) > 0 {
		return member{kind: memberDeclaration, statement: statement, names: names}
	}
	return member{kind: memberOther, statement: statement}
}

// suite is a `describe` call and the members of its callback.
type suite struct {
	call    call
	members []member
}

// testCase is a test and the suites it is nested in, outermost first.
type testCase struct {
	test   call
	suites []suite
	// pattern matches the names a runner reports for the test.
	pattern *regexp.Regexp
	// wildcards counts the placeholders of its titles: the fewer, the more
	// specific the match.
	wildcards int
	// words are the titles of the suites and of the test, which no masked line may
	// spell.
	words string
}

// testCases lists every test of the file with its suites.
func (x *fileIndex) testCases() []testCase {
	var found []testCase
	x.collect(x.membersOf(x.Root()), nil, &found)
	return found
}

func (x *fileIndex) collect(members []member, path []suite, out *[]testCase) {
	for _, m := range members {
		switch m.kind {
		case memberTest:
			*out = append(*out, x.newTestCase(m.call, path))
		case memberSuite:
			body := x.suiteBlock(m.call)
			if body == nil {
				continue
			}
			inner := suite{call: m.call, members: x.membersOf(body)}
			x.collect(inner.members, append(slices.Clone(path), inner), out)
		}
	}
}

func (x *fileIndex) newTestCase(test call, path []suite) testCase {
	var patterns, words []string
	for _, enclosing := range path {
		patterns = append(patterns, x.titlePattern(enclosing.call))
		words = append(words, x.titleWords(enclosing.call))
	}
	patterns = append(patterns, x.titlePattern(test))
	words = append(words, x.titleWords(test))
	patterns = slices.DeleteFunc(patterns, func(p string) bool { return p == "" })
	joined := strings.Join(patterns, `\s+`)
	return testCase{
		test: test, suites: path,
		pattern:   regexp.MustCompile("^" + joined + "$"),
		wildcards: strings.Count(joined, ".*?") + strings.Count(joined, wholeTitle),
		words:     strings.Join(slices.DeleteFunc(words, func(w string) bool { return w == "" }), " "),
	}
}

// distanceTo is how far the line a report gives is from the test or the suite that
// carries the title the report was read from.
func (c testCase) distanceTo(line int) int {
	if line == 0 {
		return 0
	}
	first, last := lineSpan(c.test)
	if line >= first && line <= last {
		return 0
	}
	best := min(abs(line-first), abs(line-last))
	for _, enclosing := range c.suites {
		start, _ := lineSpan(enclosing.call)
		best = min(best, abs(line-start))
	}
	return best
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// locate finds the test a report names: the one whose suites and title expand to
// the reported name. Several tests may match when a title is built from a table
// row; the most specific title wins, then the one closest to the reported line.
func (x *fileIndex) locate(bound criterion.BoundTest) (testCase, error) {
	name := testname.Compact(bound.Name)
	var matches []testCase
	for _, candidate := range x.testCases() {
		if candidate.pattern.MatchString(name) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return testCase{}, fmt.Errorf("%w: %s in %s", ports.ErrTestNotFound, bound.Label(), bound.File)
	}
	slices.SortStableFunc(matches, func(a, b testCase) int {
		return cmp.Or(cmp.Compare(a.wildcards, b.wildcards), cmp.Compare(a.distanceTo(bound.Line), b.distanceTo(bound.Line)))
	})
	return matches[0], nil
}
