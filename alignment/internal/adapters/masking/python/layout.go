package python

import "strings"

const (
	// unitSeparator sits between two top-level blocks of the unit.
	unitSeparator = "\n\n"
	// windowsLineBreak is normalised to a line feed.
	windowsLineBreak = "\r\n"
	lineSeparator    = "\n"
	trailingBlanks   = " \t\r"
)

// tidy strips trailing whitespace, collapses runs of blank lines to one and
// trims blank lines at both ends of a block.
func tidy(block string) string {
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

// join lays blocks out one blank line apart and ends the text with a single
// line break. Empty blocks are skipped.
func join(blocks []string) string {
	nonEmpty := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block != "" {
			nonEmpty = append(nonEmpty, block)
		}
	}
	return strings.Join(nonEmpty, unitSeparator) + lineSeparator
}

// indentationOf is the number of columns that precede the offset on its line.
func indentationOf(source []byte, offset int) int {
	lineStart := offset
	for lineStart > 0 && source[lineStart-1] != lineFeed {
		lineStart--
	}
	return offset - lineStart
}

// dedent removes up to columns of indentation from every line after the first,
// which already starts at the statement.
func dedent(text string, columns int) string {
	lines := strings.Split(text, lineSeparator)
	for index := 1; index < len(lines); index++ {
		lines[index] = trimIndent(lines[index], columns)
	}
	return strings.Join(lines, lineSeparator)
}

func trimIndent(line string, columns int) string {
	removed := 0
	for removed < columns && removed < len(line) && (line[removed] == space || line[removed] == tab) {
		removed++
	}
	return line[removed:]
}

// blanksBefore is the number of blank characters that directly precede the
// offset on its line.
func blanksBefore(source []byte, offset int) int {
	count := 0
	for offset-count > 0 && (source[offset-count-1] == space || source[offset-count-1] == tab) {
		count++
	}
	return count
}
