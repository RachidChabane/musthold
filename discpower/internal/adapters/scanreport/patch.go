package scanreport

import (
	"bytes"
	"fmt"
	"strings"
)

// Shape of a unified diff.
const (
	contextLines   = 3
	oldFilePrefix  = "--- a/"
	newFilePrefix  = "+++ b/"
	hunkFormat     = "@@ -%d,%d +%d,%d @@\n"
	contextMarker  = " "
	removedMarker  = "-"
	addedMarker    = "+"
	noNewlineAtEOF = "\\ No newline at end of file\n"
	newline        = "\n"
)

// UnifiedDiff writes the change of one file as a unified diff with one hunk:
// every line between the first and the last that differ, with three lines of
// context around them. `git apply` and `patch -p1` read it. Identical contents
// give no diff.
func UnifiedDiff(path string, before, after []byte) string {
	old, updated := splitLines(before), splitLines(after)
	prefix, suffix := commonEnds(old, updated)
	if prefix == len(old) && prefix == len(updated) {
		return ""
	}
	start := max(prefix-contextLines, 0)
	oldEnd := min(len(old)-suffix+contextLines, len(old))
	newEnd := min(len(updated)-suffix+contextLines, len(updated))
	var out strings.Builder
	out.WriteString(oldFilePrefix + path + newline)
	out.WriteString(newFilePrefix + path + newline)
	fmt.Fprintf(&out, hunkFormat, hunkStart(start, oldEnd), oldEnd-start, hunkStart(start, newEnd), newEnd-start)
	for index := start; index < prefix; index++ {
		writeLine(&out, contextMarker, old[index])
	}
	for index := prefix; index < len(old)-suffix; index++ {
		writeLine(&out, removedMarker, old[index])
	}
	for index := prefix; index < len(updated)-suffix; index++ {
		writeLine(&out, addedMarker, updated[index])
	}
	for index := len(old) - suffix; index < oldEnd; index++ {
		writeLine(&out, contextMarker, old[index])
	}
	return out.String()
}

// hunkStart is the first line of a hunk side, or 0 for an empty side.
func hunkStart(start, end int) int {
	if end == start {
		return start
	}
	return start + 1
}

// splitLines splits content into lines that keep their line break, so that a
// last line without one is told apart.
func splitLines(content []byte) []string {
	var lines []string
	for len(content) > 0 {
		end := bytes.IndexByte(content, '\n')
		if end < 0 {
			lines = append(lines, string(content))
			break
		}
		lines = append(lines, string(content[:end+1]))
		content = content[end+1:]
	}
	return lines
}

func writeLine(out *strings.Builder, marker, line string) {
	out.WriteString(marker + line)
	if !strings.HasSuffix(line, newline) {
		out.WriteString(newline + noNewlineAtEOF)
	}
}

// changedLines are the lines a diff removes and the lines it adds, without
// their markers and line breaks.
func changedLines(before, after []byte) (removed, added []string) {
	old, updated := splitLines(before), splitLines(after)
	prefix, suffix := commonEnds(old, updated)
	for _, line := range old[prefix : len(old)-suffix] {
		removed = append(removed, strings.TrimRight(line, "\r\n"))
	}
	for _, line := range updated[prefix : len(updated)-suffix] {
		added = append(added, strings.TrimRight(line, "\r\n"))
	}
	return removed, added
}

// commonEnds counts the lines two versions share at their start and, of the
// rest, at their end.
func commonEnds(old, updated []string) (prefix, suffix int) {
	for prefix < len(old) && prefix < len(updated) && old[prefix] == updated[prefix] {
		prefix++
	}
	for suffix < len(old)-prefix && suffix < len(updated)-prefix && old[len(old)-1-suffix] == updated[len(updated)-1-suffix] {
		suffix++
	}
	return prefix, suffix
}
