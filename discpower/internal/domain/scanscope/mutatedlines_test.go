package scanscope_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

const verifiesMutatedLines = " [verifies SP-2234~1]"

func linesOf(file string, numbers ...int) []vacuity.Line {
	var lines []vacuity.Line
	for _, number := range numbers {
		lines = append(lines, vacuity.Line{File: file, Line: number})
	}
	return lines
}

func TestMutatedLines(t *testing.T) {
	const file = "m.py"
	before := []byte("def f(x):\n    if x > 0:\n        return 1\n    return 0\n")
	cases := []struct {
		name  string
		after string
		want  []vacuity.Line
	}{
		{"a changed line is the line it edits", "def f(x):\n    if x >= 0:\n        return 1\n    return 0\n", linesOf(file, 2)},
		{"two changed lines edit every line between them", "def f(x):\n    if x >= 0:\n        return 2\n    return 0\n", linesOf(file, 2, 3)},
		{"a deleted line is the line it edits", "def f(x):\n    if x > 0:\n    return 0\n", linesOf(file, 3)},
		{"an inserted line edits the line it comes before", "def f(x):\n    print(x)\n    if x > 0:\n        return 1\n    return 0\n", linesOf(file, 2)},
		{"an appended line edits the last line", "def f(x):\n    if x > 0:\n        return 1\n    return 0\n# end\n", linesOf(file, 4)},
		{"an unchanged file edits nothing", string(before), nil},
	}
	for _, c := range cases {
		t.Run(c.name+verifiesMutatedLines, func(t *testing.T) {
			if got := scanscope.MutatedLines(file, before, []byte(c.after)); !slices.Equal(got, c.want) {
				t.Fatalf("lines %v, want %v", got, c.want)
			}
		})
	}
}
