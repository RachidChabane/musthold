// Package testname reads the names a test runner reports for tests whose titles
// carry a requirement citation in square brackets: it takes the citations and the
// runner's separators out, so that what remains is the name its author
// wrote.
package testname

import (
	"regexp"
	"strings"
)

// citationPattern matches a citation tag with the spaces before it. The marker
// word is spelled in two parts so that no line of this source reads as a tag.
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

// StripCitations removes every citation tag from the text.
func StripCitations(text string) string { return citationPattern.ReplaceAllString(text, "") }

// Compact removes the citation tags of the name, unifies the separators between
// nested titles into single spaces and trims the result.
func Compact(name string) string {
	return strings.TrimSpace(separatorPattern.ReplaceAllString(StripCitations(name), space))
}

// PatternOf turns a title into the regular expression source that matches the
// names it can expand to: every placeholder the title holds matches any text,
// the rest of the title matches itself, and any run of white space matches any
// run of white space.
func PatternOf(title string, placeholders *regexp.Regexp) string {
	var out strings.Builder
	cursor := 0
	for _, found := range placeholders.FindAllStringIndex(title, allMatches) {
		out.WriteString(quoteWords(title[cursor:found[0]]))
		out.WriteString(anyText)
		cursor = found[1]
	}
	out.WriteString(quoteWords(title[cursor:]))
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
