package task

import (
	"fmt"
	"strings"

	l1accept "github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
)

// The size of a fixture, in counted lines.
const (
	MinLines    = 250
	MaxLines    = 600
	MinMaxLines = 400
)

// MinLinesOf is the least size of a fixture of a tier.
func MinLinesOf(tier Tier) int {
	if tier == TierMax {
		return MinMaxLines
	}
	return MinLines
}

// Lines counts the size of a fixture: the non-blank lines of its Python files
// and of its spec.
func Lines(fixture map[string][]byte, spec string) int {
	count := 0
	for name, content := range fixture {
		if name != spec && !strings.HasSuffix(name, l1accept.PythonSuffix) {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			if strings.TrimSpace(line) != "" {
				count++
			}
		}
	}
	return count
}

// SizeProblem says how a fixture's size departs from its tier's shape, or is
// empty.
func SizeProblem(lines int, tier Tier) string {
	if least := MinLinesOf(tier); lines < least || lines > MaxLines {
		return fmt.Sprintf("the project holds %d lines of Python and spec, a %s task holds %d to %d", lines, tier, least, MaxLines)
	}
	return ""
}
