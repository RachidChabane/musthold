package scanscope

import (
	"bytes"
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

// lineBreak ends a line of source.
const lineBreak = '\n'

// SpanLines lists the lines of a file a span of its bytes touches: every line
// from the one that holds its first byte to the one that holds its last. An
// empty span, an insertion, touches the line it is inserted at. A span outside
// the content touches nothing.
func SpanLines(file string, content []byte, span ecosystem.Span) []vacuity.Line {
	if !span.Within(len(content)) {
		return nil
	}
	first := 1 + bytes.Count(content[:span.Start], []byte{lineBreak})
	last := first
	if span.Len() > 0 {
		last = first + bytes.Count(content[span.Start:span.End-1], []byte{lineBreak})
	}
	lines := make([]vacuity.Line, 0, last-first+1)
	for line := first; line <= last; line++ {
		lines = append(lines, vacuity.Line{File: file, Line: line})
	}
	return lines
}

// LineText is the text of one line of the content, without its line break; a
// line outside the content is empty.
func LineText(content []byte, line int) string {
	for number, text := range bytes.Split(content, []byte{lineBreak}) {
		if number+1 == line {
			return string(bytes.TrimSuffix(text, []byte{'\r'}))
		}
	}
	return ""
}

// LineSet is a set of lines.
type LineSet map[vacuity.Line]bool

// NewLineSet holds the given lines.
func NewLineSet(lines ...[]vacuity.Line) LineSet {
	set := LineSet{}
	for _, group := range lines {
		for _, line := range group {
			set[line] = true
		}
	}
	return set
}

// Intersects reports whether one of the lines is in the set.
func (s LineSet) Intersects(lines []vacuity.Line) bool {
	return slices.ContainsFunc(lines, func(line vacuity.Line) bool { return s[line] })
}

// Files lists the files of the set, sorted.
func (s LineSet) Files() []string {
	var files []string
	for line := range s {
		if !slices.Contains(files, line.File) {
			files = append(files, line.File)
		}
	}
	slices.Sort(files)
	return files
}

// Sorted lists the lines of the set by file, then number.
func (s LineSet) Sorted() []vacuity.Line {
	lines := make([]vacuity.Line, 0, len(s))
	for line := range s {
		lines = append(lines, line)
	}
	slices.SortFunc(lines, format.CompareScanLines)
	return lines
}

// DefinitionLines lists, per file, the lines of every definition that holds one
// of the given lines: the functions a change edits, whole.
func DefinitionLines(file string, definitions []ecosystem.Definition, changed []int) []vacuity.Line {
	set := LineSet{}
	for _, definition := range definitions {
		if definition.Kind == ecosystem.KindAssignment {
			continue
		}
		if !slices.ContainsFunc(changed, func(line int) bool { return line >= definition.StartLine && line <= definition.EndLine }) {
			continue
		}
		for line := definition.StartLine; line <= definition.EndLine; line++ {
			set[vacuity.Line{File: file, Line: line}] = true
		}
	}
	return set.Sorted()
}
