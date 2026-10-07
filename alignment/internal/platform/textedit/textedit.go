// Package textedit rewrites source text by byte spans: it applies a set of
// replacements to a range of a file and tidies the result, which is how a masker
// removes comments, titles and markers from code it otherwise leaves untouched.
package textedit

import (
	"slices"
	"strings"
)

// Characters of the layout the package reads and writes.
const (
	LineFeed       = '\n'
	CarriageReturn = '\r'
	Space          = ' '
	Tab            = '\t'

	lineSeparator    = "\n"
	windowsLineBreak = "\r\n"
	trailingBlanks   = " \t\r"
	blockSeparator   = "\n\n"
)

// Span is a byte range of a source file.
type Span struct{ Start, End int }

// Contains reports whether the other span lies inside the span.
func (s Span) Contains(other Span) bool { return other.Start >= s.Start && other.End <= s.End }

// Edit replaces a byte range of the source.
type Edit struct {
	Span        Span
	Replacement string
}

// Render returns the source text of the span with the edits that fall inside it
// applied. An edit nested in an earlier one is dropped, and one that starts
// inside an earlier one is cut to what lies beyond it.
func Render(source []byte, within Span, edits []Edit) string {
	inside := make([]Edit, 0, len(edits))
	for _, candidate := range edits {
		if within.Contains(candidate.Span) {
			inside = append(inside, candidate)
		}
	}
	slices.SortStableFunc(inside, compareEdits)

	var out strings.Builder
	cursor := within.Start
	for _, applied := range inside {
		if applied.Span.End <= cursor && applied.Span.Start < cursor {
			continue
		}
		applied.Span.Start = max(applied.Span.Start, cursor)
		out.Write(source[cursor:applied.Span.Start])
		out.WriteString(applied.Replacement)
		cursor = applied.Span.End
	}
	out.Write(source[cursor:within.End])
	return out.String()
}

// compareEdits orders edits by start, the longer first, so an outer edit wins
// over the edits nested in it.
func compareEdits(a, b Edit) int {
	if a.Span.Start != b.Span.Start {
		return a.Span.Start - b.Span.Start
	}
	return b.Span.End - a.Span.End
}

// Tidy strips trailing whitespace, collapses runs of blank lines to one and trims
// blank lines at both ends of a block.
func Tidy(block string) string {
	lines := strings.Split(strings.ReplaceAll(block, windowsLineBreak, lineSeparator), lineSeparator)
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, trailingBlanks)
		if line == "" && (len(kept) == 0 || kept[len(kept)-1] == "") {
			continue
		}
		kept = append(kept, line)
	}
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}
	return strings.Join(kept, lineSeparator)
}

// Join lays blocks out one blank line apart and ends the text with a single line
// break. Empty blocks are skipped.
func Join(blocks []string) string {
	nonEmpty := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block != "" {
			nonEmpty = append(nonEmpty, block)
		}
	}
	return strings.Join(nonEmpty, blockSeparator) + lineSeparator
}

// IndentationOf is the number of columns that precede the offset on its line.
func IndentationOf(source []byte, offset int) int {
	lineStart := offset
	for lineStart > 0 && source[lineStart-1] != LineFeed {
		lineStart--
	}
	return offset - lineStart
}

// BlanksBefore is the number of blank characters that directly precede the offset
// on its line.
func BlanksBefore(source []byte, offset int) int {
	count := 0
	for offset-count > 0 && (source[offset-count-1] == Space || source[offset-count-1] == Tab) {
		count++
	}
	return count
}

// SkipBlanks returns the offset of the first character at or after from that is
// not a space or a tab.
func SkipBlanks(source []byte, from int) int {
	for from < len(source) && (source[from] == Space || source[from] == Tab) {
		from++
	}
	return from
}

// LineEnd is the offset after the line break that ends the line holding offset.
func LineEnd(source []byte, offset int) int {
	for offset < len(source) && source[offset] != LineFeed {
		offset++
	}
	return min(offset+1, len(source))
}

// Dedent removes up to columns of indentation from every line after the first,
// which already starts where its statement does.
func Dedent(text string, columns int) string { return dedentFrom(text, columns, 1) }

// DedentAll removes up to columns of indentation from every line.
func DedentAll(text string, columns int) string { return dedentFrom(text, columns, 0) }

func dedentFrom(text string, columns, first int) string {
	lines := strings.Split(text, lineSeparator)
	for index := first; index < len(lines); index++ {
		lines[index] = trimIndent(lines[index], columns)
	}
	return strings.Join(lines, lineSeparator)
}

// LineStart is the offset of the start of the line that holds the offset when only
// blanks precede it on its line, and the offset itself otherwise.
func LineStart(source []byte, offset int) int {
	if IndentationOf(source, offset) == BlanksBefore(source, offset) {
		return offset - BlanksBefore(source, offset)
	}
	return offset
}

func trimIndent(line string, columns int) string {
	removed := 0
	for removed < columns && removed < len(line) && (line[removed] == Space || line[removed] == Tab) {
		removed++
	}
	return line[removed:]
}

// Indent prefixes every non-empty line of the text with the indentation.
func Indent(text, indentation string) string {
	lines := strings.Split(text, lineSeparator)
	for index, line := range lines {
		if line != "" {
			lines[index] = indentation + line
		}
	}
	return strings.Join(lines, lineSeparator)
}

// RemoveWholeLines returns the edit that removes the lines the span occupies,
// which hold nothing else.
func RemoveWholeLines(source []byte, span Span) Edit {
	start := span.Start - IndentationOf(source, span.Start)
	return Edit{Span: Span{Start: start, End: LineEnd(source, span.End)}}
}

// RemoveText returns the edit that removes the span, and its whole line when
// nothing else is on it.
func RemoveText(source []byte, span Span) Edit {
	if IndentationOf(source, span.Start) != BlanksBefore(source, span.Start) {
		return Edit{Span: span}
	}
	after := SkipBlanks(source, span.End)
	if after < len(source) && source[after] == CarriageReturn {
		after++
	}
	if after < len(source) && source[after] != LineFeed {
		return Edit{Span: span}
	}
	return RemoveWholeLines(source, span)
}
