// Package codexconfig reads the user's Codex configuration file as far as the
// canary needs: it finds the table Codex persists for a trusted project and takes
// exactly that table out. It is pure: it reads and writes no file.
package codexconfig

import (
	"bytes"
	"strconv"
	"strings"
)

const (
	projectsTable   = "projects."
	tableOpen       = "["
	arrayTableOpen  = "[["
	tableClose      = "]"
	commentMark     = "#"
	literalQuote    = "'"
	newline         = "\n"
	lineEndingBytes = "\r"
)

// Removal is the outcome of taking a project's table out of a configuration.
type Removal struct {
	// Found says the configuration holds the table.
	Found bool
	// Table is the text of the table that was removed: its header and its keys.
	Table string
	// Cleaned is the configuration without the table; the input itself when the
	// table is not there.
	Cleaned []byte
}

// RemoveProjectTable removes the table `[projects."<dir>"]` and its keys, and
// nothing else. When the table ends the file, the one blank line that Codex puts
// before a table it appends goes with it.
func RemoveProjectTable(content []byte, projectDir string) Removal {
	lines := splitLines(content)
	start := tableStart(lines, projectDir)
	if start < 0 {
		return Removal{Cleaned: content}
	}
	end := tableEnd(lines, start)
	table := strings.Join(lines[start:end], "")
	kept := append(append([]string(nil), lines[:start]...), lines[end:]...)
	if end == len(lines) {
		kept = dropOneTrailingBlankLine(kept)
	}
	return Removal{Found: true, Table: table, Cleaned: []byte(strings.Join(kept, ""))}
}

// NamesProject reports whether the configuration mentions the project directory
// anywhere, in a table or a key.
func NamesProject(content []byte, projectDir string) bool {
	return bytes.Contains(content, []byte(projectDir))
}

// splitLines splits after every newline, so that joining the parts gives back the
// input byte for byte.
func splitLines(content []byte) []string {
	return strings.SplitAfter(string(content), newline)
}

func tableStart(lines []string, projectDir string) int {
	for index, line := range lines {
		if isProjectHeader(line, projectDir) {
			return index
		}
	}
	return -1
}

// isProjectHeader reports whether the line is the header of the project's table,
// written with a basic or a literal quoted key, and possibly followed by a comment.
func isProjectHeader(line, projectDir string) bool {
	text := strings.TrimSpace(line)
	for _, header := range []string{
		tableOpen + projectsTable + strconv.Quote(projectDir) + tableClose,
		tableOpen + projectsTable + literalQuote + projectDir + literalQuote + tableClose,
	} {
		if rest, found := strings.CutPrefix(text, header); found {
			rest = strings.TrimSpace(rest)
			return rest == "" || strings.HasPrefix(rest, commentMark)
		}
	}
	return false
}

// tableEnd is the index of the line after the table that starts at start: the next
// table header, or the end of the file.
func tableEnd(lines []string, start int) int {
	for index := start + 1; index < len(lines); index++ {
		text := strings.TrimSpace(lines[index])
		if strings.HasPrefix(text, tableOpen) || strings.HasPrefix(text, arrayTableOpen) {
			return index
		}
	}
	return len(lines)
}

// dropOneTrailingBlankLine removes the blank line a file ends with, when it
// ends with one.
func dropOneTrailingBlankLine(lines []string) []string {
	last := len(lines) - 1
	for last >= 0 && lines[last] == "" {
		last--
	}
	if last >= 0 && strings.TrimRight(lines[last], lineEndingBytes+newline) == "" {
		return append(lines[:last:last], lines[last+1:]...)
	}
	return lines
}
