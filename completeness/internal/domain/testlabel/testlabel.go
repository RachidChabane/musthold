// Package testlabel reads the labels a test runner reports for tests whose titles
// carry a requirement citation in square brackets, and builds the patterns that
// match them: it takes the citations and the separators between nested titles out,
// so that a label is compared by the words its author wrote.
package testlabel

import (
	"regexp"
	"strings"
)

// citationPattern matches a citation tag with the spaces before it. The marker word
// is spelled in two parts so that no line of this source reads as a tag.
var citationPattern = regexp.MustCompile(`(?i)\s*\[\s*verif` + `ies\b[^\]]*\]`)

// separatorPattern matches the separators runners put between the titles of a
// nested test: a space, or ` > ` as Vitest's reporter writes it.
var separatorPattern = regexp.MustCompile(`\s*>\s+|\s+`)

const (
	space      = " "
	allMatches = -1
	anyText    = `.*?`
	anySpace   = `\s*`
	someSpace  = `\s+`
	blanks     = " \t\r\n"
)

// Compact removes the citation tags of the label, unifies the separators between
// nested titles into single spaces and trims the result.
func Compact(label string) string {
	return strings.TrimSpace(separatorPattern.ReplaceAllString(citationPattern.ReplaceAllString(label, ""), space))
}

// PatternOf turns a title into the regular expression source that matches the
// labels it can expand to: every placeholder the title holds matches any text, the
// rest of the title matches itself, and any run of white space matches any run of
// white space. The citations of the title are taken out first.
func PatternOf(title string, placeholders *regexp.Regexp) string {
	compact := Compact(title)
	var out strings.Builder
	cursor := 0
	for _, found := range placeholders.FindAllStringIndex(compact, allMatches) {
		out.WriteString(quoteWords(compact[cursor:found[0]]))
		out.WriteString(anyText)
		cursor = found[1]
	}
	out.WriteString(quoteWords(compact[cursor:]))
	return out.String()
}

func quoteWords(text string) string {
	words := strings.Fields(text)
	quoted := make([]string, len(words))
	for index, word := range words {
		quoted[index] = regexp.QuoteMeta(word)
	}
	prefix, suffix := "", ""
	if text != "" && strings.TrimLeft(text, blanks) != text {
		prefix = anySpace
	}
	if text != "" && strings.TrimRight(text, blanks) != text {
		suffix = anySpace
	}
	return prefix + strings.Join(quoted, someSpace) + suffix
}

// Matches reports whether any of the patterns matches the label once its citations
// are taken out. A pattern that is not a valid expression matches nothing.
func Matches(patterns []string, label string) bool {
	compact := Compact(label)
	for _, source := range patterns {
		if pattern, err := regexp.Compile(source); err == nil && pattern.MatchString(compact) {
			return true
		}
	}
	return false
}

// JUnitClass is a class a JUnit 5 test is declared in.
type JUnitClass struct {
	Name string
	// Display is the display name of the class, empty when it has none.
	Display string
}

// JUnitTest is what the source says about a test method of JUnit 5: the names a
// runner reports it under.
type JUnitTest struct {
	Method string
	// Display is the display name of the method and Template the name pattern of a
	// parametrized or repeated test, empty when there are none.
	Display, Template string
	// Parameterized and Repeated tell how the method is run.
	Parameterized, Repeated bool
	// Classes are the classes the method is declared in, outermost first.
	Classes []JUnitClass
}

// Pieces of the labels JUnit 5 gives its tests.
const (
	classBoundary  = `(?:^|[.$\s])`
	classRemainder = `[^.]*\.`
	nameEnd        = `$`
	methodTail     = `(?:\([^)]*\))?(?:\[\d+\].*)?`
	invocationName = `\[\d+\].*`
	repetitionName = `repetition\s+\d+.*`
	alternation    = "|"
)

var junitPlaceholders = regexp.MustCompile(`\{[^}]*\}`)

// JUnitPattern is the regular expression source that matches the label a runner
// reports for a JUnit 5 test, `classname.name`, once its citations are taken out:
// the class is its binary name, or the display names of its classes, and the name
// is the method name with its signature, the display name, the expansion of the
// name pattern or the default name of an invocation or a repetition.
func JUnitPattern(test JUnitTest) string {
	var classes []string
	if len(test.Classes) > 0 {
		innermost := test.Classes[len(test.Classes)-1]
		classes = append(classes, regexp.QuoteMeta(innermost.Name))
		if innermost.Display != "" {
			classes = append(classes, regexp.QuoteMeta(Compact(innermost.Display)))
		}
	}
	names := []string{regexp.QuoteMeta(test.Method) + methodTail}
	if test.Display != "" {
		names = append(names, PatternOf(test.Display, junitPlaceholders))
	}
	switch {
	case test.Template != "":
		names = append(names, PatternOf(test.Template, junitPlaceholders))
	case test.Parameterized:
		names = append(names, invocationName)
	case test.Repeated:
		names = append(names, repetitionName)
	}
	return classBoundary + "(?:" + strings.Join(classes, alternation) + ")" + classRemainder + "(?:" + strings.Join(names, alternation) + ")" + nameEnd
}
