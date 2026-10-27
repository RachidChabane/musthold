package freeze

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// ErrSums is returned for a sums file that is not in shasum's format.
var ErrSums = errors.New("malformed sums file")

// Sum is one line of a sums file: a file's SHA-256 and its repository path.
type Sum struct {
	Digest string
	Path   string
}

const sumSeparator = "  "

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// FormatSums renders sums in shasum's format, sorted by path.
func FormatSums(sums []Sum) string {
	sorted := slices.Clone(sums)
	slices.SortFunc(sorted, func(a, b Sum) int { return strings.Compare(a.Path, b.Path) })
	var text strings.Builder
	for _, sum := range sorted {
		text.WriteString(sum.Digest + sumSeparator + sum.Path + "\n")
	}
	return text.String()
}

// ParseSums reads a sums file.
func ParseSums(text string) ([]Sum, error) {
	var sums []Sum
	for number, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		digest, file, found := strings.Cut(line, sumSeparator)
		if !found || !digestPattern.MatchString(digest) || file == "" {
			return nil, fmt.Errorf("%w: line %d", ErrSums, number+1)
		}
		sums = append(sums, Sum{Digest: digest, Path: file})
	}
	return sums, nil
}
