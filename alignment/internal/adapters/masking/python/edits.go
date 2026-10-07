package python

import (
	"slices"
	"strings"
)

// span is a byte range of a source file.
type span struct{ start, end int }

func (s span) contains(other span) bool { return other.start >= s.start && other.end <= s.end }

// edit replaces a byte range of the source.
type edit struct {
	span        span
	replacement string
}

// render returns the source text of the span with the edits that fall inside it
// applied. An edit nested in an earlier one is dropped, and one that starts
// inside an earlier one is cut to what lies beyond it.
func render(source []byte, within span, edits []edit) string {
	inside := make([]edit, 0, len(edits))
	for _, candidate := range edits {
		if within.contains(candidate.span) {
			inside = append(inside, candidate)
		}
	}
	slices.SortStableFunc(inside, compareEdits)

	var out strings.Builder
	cursor := within.start
	for _, applied := range inside {
		if applied.span.end <= cursor && applied.span.start < cursor {
			continue
		}
		applied.span.start = max(applied.span.start, cursor)
		out.Write(source[cursor:applied.span.start])
		out.WriteString(applied.replacement)
		cursor = applied.span.end
	}
	out.Write(source[cursor:within.end])
	return out.String()
}

// compareEdits orders edits by start, the longer first, so an outer edit wins
// over the edits nested in it.
func compareEdits(a, b edit) int {
	if a.span.start != b.span.start {
		return a.span.start - b.span.start
	}
	return b.span.end - a.span.end
}
