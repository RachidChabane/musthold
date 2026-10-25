package record

import (
	"bytes"
	"regexp"
	"slices"
)

// tokenPattern is the shape of a Claude Code token, which a record must never
// hold whatever its value.
var tokenPattern = regexp.MustCompile(`sk-ant-[a-z0-9]+-[A-Za-z0-9_-]{20,}`)

// minimumExact is the shortest exact value the scan looks for: anything shorter
// would match by chance.
const minimumExact = 16

// TokenHits lists, sorted, the files that hold a token: the exact value when one
// is given, or any text of a token's shape. It reports names, never what matched.
func TokenHits(files map[string][]byte, exact string) []string {
	var hits []string
	for name, content := range files {
		if tokenPattern.Match(content) || (len(exact) >= minimumExact && bytes.Contains(content, []byte(exact))) {
			hits = append(hits, name)
		}
	}
	slices.Sort(hits)
	return hits
}
