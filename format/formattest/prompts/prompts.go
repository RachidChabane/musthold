// Package prompts is test support: it reads the parts of the prompts the proof
// modules write that an honest host of a test project answers from, the code
// slice of a generation prompt, the requirements of a screening prompt and the
// sections a header starts.
package prompts

import (
	"regexp"
	"strings"
)

// Markers of the prompts the readers look for.
const (
	codeHeader     = "Code under test:\n"
	codeEnd        = "\n\nWrite up to"
	unitSeparator  = "--- "
	sectionEnd     = "\n\n"
	lineEnd        = "\n"
	requirementKey = 1
	statementKey   = 2
)

var (
	unitHeader       = regexp.MustCompile(`^(\S+) :: (\S+)\n`)
	requirementBlock = regexp.MustCompile(`(?s)Requirement (\d+):\n(.*?)\n\n`)
)

// Unit is one unit of the code slice a generation prompt shows: the file it is
// in and its source.
type Unit struct {
	File, Source string
}

// Units reads the units of the code slice a generation prompt shows, by
// qualified name. A prompt without a code slice has none.
func Units(user string) map[string]Unit {
	units := map[string]Unit{}
	start := strings.Index(user, codeHeader)
	end := strings.Index(user, codeEnd)
	if start < 0 || end < start {
		return units
	}
	for _, block := range strings.Split(user[start+len(codeHeader):end], unitSeparator)[1:] {
		header := unitHeader.FindStringSubmatch(block)
		if header == nil {
			continue
		}
		units[header[2]] = Unit{File: header[1], Source: strings.TrimRight(block[len(header[0]):], lineEnd)}
	}
	return units
}

// Requirement is one requirement a screening prompt asks about.
type Requirement struct {
	// Number is the requirement's number in the prompt, which its question id carries.
	Number    string
	Statement string
}

// Requirements reads the requirements a screening prompt lists, in order.
func Requirements(user string) []Requirement {
	var found []Requirement
	for _, match := range requirementBlock.FindAllStringSubmatch(user, -1) {
		found = append(found, Requirement{Number: match[requirementKey], Statement: match[statementKey]})
	}
	return found
}

// LineAfter returns the rest of the line a header starts.
func LineAfter(text, header string) string {
	start := strings.Index(text, header)
	if start < 0 {
		return ""
	}
	line, _, _ := strings.Cut(text[start+len(header):], lineEnd)
	return line
}

// SectionAfter returns the text a header starts, up to the next blank line.
func SectionAfter(text, header string) string {
	start := strings.Index(text, header)
	if start < 0 {
		return ""
	}
	section, _, _ := strings.Cut(text[start:], sectionEnd)
	return section
}
