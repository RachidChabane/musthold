// Package junit finds the JUnit 5 test method a report names. The name a runner
// reports for a test is the method name, its display name, or the name pattern of
// a parametrized or repeated test with its placeholders expanded, and its class is
// spelled as a binary name, as `Outer$Inner`, or as the display names of the
// classes; the package reads all of them against the tests found in the source.
package junit

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/testname"
)

// Kind tells how a test method is run.
type Kind int

// Kinds of test method.
const (
	// Plain is a method run once, as `@Test`.
	Plain Kind = iota
	// Parameterized is a method run once per argument set, as `@ParameterizedTest`.
	Parameterized
	// Repeated is a method run several times, as `@RepeatedTest`.
	Repeated
	// Dynamic is a method that returns the tests it runs, as `@TestFactory`.
	Dynamic
)

// Class is a class a test is declared in.
type Class struct {
	Name string
	// Display is the display name of the class, empty when it has none.
	Display string
	Line    int
}

// Test is a test method as the source declares it.
type Test struct {
	// Method is the name of the method, without the backticks of a Kotlin name.
	Method string
	// Display is the display name of the method, empty when it has none.
	Display string
	// Template is the name pattern of a parametrized or repeated test, empty when
	// it has none.
	Template string
	Kind     Kind
	// Classes are the classes the method is declared in, outermost first.
	Classes []Class
	// StartLine and EndLine are the first and last line of the method with its
	// annotations.
	StartLine, EndLine int
}

// Names that JUnit gives the tests it runs when their author gave none.
var (
	// placeholders are the arguments of a name pattern, as `{0}` or `{index}`.
	placeholders = regexp.MustCompile(`\{[^}]*\}`)
	// invocationIndex starts the default name of a parametrized invocation.
	invocationIndex = regexp.MustCompile(`^\[\d+\]`)
	// repetitionName starts the default name of a repetition.
	repetitionName = regexp.MustCompile(`(?i)^repetition \d+`)
	// signatureSuffix is what follows a method name in a report: its parameter types
	// and the index of an invocation.
	signatureSuffix = regexp.MustCompile(`^(\([^)]*\))?(\[\d+\].*)?$`)
	// classSeparators splits a binary class name into the names it joins.
	classSeparators = regexp.MustCompile(`[.$]`)
)

const (
	nestedSeparator = "$"
	wordSeparator   = " "
	anchorStart     = "^"
	anchorEnd       = "$"
	bracketOpen     = "["
	bracketClose    = "]"
)

// Words lists the texts the author of the test wrote to name it, which no masked
// line may spell: the method name, the display names and the name pattern, with
// the citations and the placeholders removed.
func (t Test) Words() []string {
	words := []string{t.Method, readable(t.Display), readable(t.Template)}
	for _, class := range t.Classes {
		words = append(words, class.Name, readable(class.Display))
	}
	return slices.DeleteFunc(words, func(word string) bool { return word == "" })
}

// readable is a display text without citations and placeholders, and with no
// bracket that would cut the name short in the leak check.
func readable(text string) string {
	cleared := placeholders.ReplaceAllString(testname.StripCitations(text), wordSeparator)
	cleared = strings.NewReplacer(bracketOpen, wordSeparator, bracketClose, wordSeparator).Replace(cleared)
	return strings.TrimSpace(cleared)
}

// Locate finds the test a report names, by the name it reports, the class it
// reports and the line of its source. It returns the index in tests.
func Locate(tests []Test, name, classname string, line int) (int, bool) {
	compact := testname.Compact(name)
	var matches []int
	for index, candidate := range tests {
		if candidate.named(compact) {
			matches = append(matches, index)
		}
	}
	if len(matches) == 0 {
		return 0, false
	}
	slices.SortStableFunc(matches, func(a, b int) int {
		return cmp.Or(
			cmp.Compare(tests[b].classRank(classname), tests[a].classRank(classname)),
			cmp.Compare(tests[a].distanceTo(line), tests[b].distanceTo(line)),
		)
	})
	return matches[0], true
}

// named reports whether a runner may report the name for the test.
func (t Test) named(name string) bool {
	if t.Display != "" && matches(patternOf(t.Display), name) {
		return true
	}
	if t.Template != "" && matches(patternOf(t.Template), name) {
		return true
	}
	if t.matchesMethod(name) {
		return true
	}
	switch t.Kind {
	case Parameterized:
		return t.Template == "" && invocationIndex.MatchString(name)
	case Repeated:
		return t.Template == "" && repetitionName.MatchString(name)
	}
	return false
}

// matchesMethod reads the method name followed by its parameter types and the
// index of an invocation, as Gradle and Surefire report an unnamed test.
func (t Test) matchesMethod(name string) bool {
	rest, found := strings.CutPrefix(name, t.Method)
	return found && signatureSuffix.MatchString(rest)
}

func patternOf(title string) *regexp.Regexp {
	return regexp.MustCompile(anchorStart + testname.PatternOf(testname.Compact(title), placeholders) + anchorEnd)
}

func matches(pattern *regexp.Regexp, name string) bool { return pattern.MatchString(name) }

// classRank tells how well the class a report names fits the classes of the test:
// three for the binary name of the class or the display names of the chain, two for
// a report that holds the name or the display name of the innermost class, one for
// one that holds those of an enclosing class, zero otherwise.
func (t Test) classRank(classname string) int {
	if classname == "" || len(t.Classes) == 0 {
		return 0
	}
	compact := testname.Compact(classname)
	if binaryName(classname) == t.binaryChain() || compact == t.phrase() {
		return rankExact
	}
	innermost := len(t.Classes) - 1
	for index := innermost; index >= 0; index-- {
		class := t.Classes[index]
		if strings.Contains(compact, class.Name) || (class.Display != "" && strings.Contains(compact, testname.Compact(class.Display))) {
			if index == innermost {
				return rankInnermost
			}
			return rankEnclosing
		}
	}
	return 0
}

// Ranks of a class a report names.
const (
	rankEnclosing = iota + 1
	rankInnermost
	rankExact
)

// phrase spells the classes of the test as a runner that phrases class names does:
// the display name of each class, or its name when it has none, joined by spaces.
func (t Test) phrase() string {
	parts := make([]string, len(t.Classes))
	for index, class := range t.Classes {
		parts[index] = cmp.Or(class.Display, class.Name)
	}
	return testname.Compact(strings.Join(parts, wordSeparator))
}

// binaryChain spells the classes of the test as `Outer$Inner`.
func (t Test) binaryChain() string {
	names := make([]string, len(t.Classes))
	for index, class := range t.Classes {
		names[index] = class.Name
	}
	return strings.Join(names, nestedSeparator)
}

// binaryName is the class name of a report without its package.
func binaryName(classname string) string {
	segments := classSeparators.Split(strings.TrimSpace(classname), -1)
	for index, segment := range segments {
		if first, _ := utf8.DecodeRuneInString(segment); unicode.IsUpper(first) {
			return strings.Join(segments[index:], nestedSeparator)
		}
	}
	return classname
}

// distanceTo is how far the line of a report is from the method or from a class
// that carries the title the report was read from.
func (t Test) distanceTo(line int) int {
	if line == 0 || (line >= t.StartLine && line <= t.EndLine) {
		return 0
	}
	best := min(abs(line-t.StartLine), abs(line-t.EndLine))
	for _, class := range t.Classes {
		best = min(best, abs(line-class.Line))
	}
	return best
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
