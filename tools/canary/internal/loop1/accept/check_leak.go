package accept

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
)

// MinLeakLineLength is the length, once trimmed, from which a line of a truth
// file counts when a fixture file holds it too. Shorter lines are punctuation,
// keywords and one-word statements that every Python file shares.
const MinLeakLineLength = 20

// forbiddenWords matches the words no fixture file may hold, with the endings
// they take.
var forbiddenWords = regexp.MustCompile(`(?i)\b(?:oracle|mutant|planted|weak|deliberate)[a-z]*`)

// origin says where a truth line comes from.
type origin struct {
	file string
	line int
	role records.Role
}

// leak is check A8: no fixture file holds a line of an oracle, violation or
// refactor file, a truth path, or one of the forbidden words. A line of an oracle
// file that only restates an inline example of the spec does not count, nor does
// one that a test file of the fixture holds too (the test writer never sees the
// fixture, so two tests of one spec can coincide), and neither does a line of a
// violation or refactor that its reference file holds too, or a line shorter
// than MinLeakLineLength, or an import.
func (r *round) leak(_ context.Context) (*Result, error) {
	res := newResult(CheckLeak)
	truth := r.truthLines()
	for _, name := range sortedKeys(r.m.fixture) {
		content := r.m.fixture[name]
		if bytes.IndexByte(content, 0) >= 0 {
			continue
		}
		text := string(content)
		r.checkLines(res, name, text, truth)
		for _, word := range forbiddenWords.FindAllString(text, -1) {
			res.fail(records.RoleAuthor, records.TargetFixture, "%s holds the word %q", name, word)
		}
		for _, truthPath := range r.truthPaths() {
			if strings.Contains(text, truthPath) {
				res.fail(records.RoleAuthor, records.TargetFixture, "%s holds the path %s", name, truthPath)
			}
		}
	}
	return res, nil
}

func (r *round) truthPaths() []string {
	truth := r.m.task.Truth
	return []string{truth.Reference, truth.Violations, truth.Refactors, truth.Oracle}
}

func (r *round) checkLines(res *Result, name, text string, truth map[string][]origin) {
	reported := map[string]bool{}
	for number, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		origins := truth[line]
		if len(origins) == 0 || reported[line] {
			continue
		}
		reported[line] = true
		res.fail(records.RoleAuthor, records.TargetFixture, "line %d of %s is also a line of another file of the task: %q", number+1, name, truncate(line))
		for _, from := range origins {
			if from.role == records.RoleOracle {
				res.fail(records.RoleOracle, records.TargetOracle, "line %d of %s must be written differently", from.line, from.file)
			}
		}
	}
}

// truthLines indexes the significant lines of the oracle, violation and refactor
// files by their trimmed text.
func (r *round) truthLines() map[string][]origin {
	lines := map[string][]origin{}
	restated := r.restatedExamples()
	shared := r.fixtureTestLines()
	for _, name := range sortedKeys(r.m.oracle) {
		for number, line := range strings.Split(string(r.m.oracle[name]), "\n") {
			line = strings.TrimSpace(line)
			if significant(line) && !restated[squeeze(line)] && !shared[squeeze(line)] {
				lines[line] = append(lines[line], origin{file: name, line: number + 1, role: records.RoleOracle})
			}
		}
	}
	for _, v := range append(append([]variant(nil), r.m.violations...), r.m.refactors...) {
		for _, name := range sortedKeys(v.Edits) {
			known := lineSet(string(r.m.reference[name]))
			for number, line := range strings.Split(string(v.Edits[name]), "\n") {
				line = strings.TrimSpace(line)
				if significant(line) && !known[line] {
					lines[line] = append(lines[line], origin{file: v.Label() + "/" + name, line: number + 1, role: records.RoleAuthor})
				}
			}
		}
	}
	return lines
}

// fixtureTestLines lists the lines of the fixture's own test files, with their
// spaces removed.
func (r *round) fixtureTestLines() map[string]bool {
	lines := map[string]bool{}
	for name, content := range r.m.fixture {
		if !strings.HasPrefix(name, testsDirectory+"/") || !strings.HasSuffix(name, pythonSuffix) {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			lines[squeeze(line)] = true
		}
	}
	return lines
}

// restatedExamples lists the assertion lines that restate an inline example.
func (r *round) restatedExamples() map[string]bool {
	restated := map[string]bool{}
	operation := r.m.task.Interface.Operation
	for _, examples := range ParseExamples(r.m.fixture[r.m.task.Spec]) {
		for _, example := range examples {
			restated[squeeze(fmt.Sprintf("assert %s(%s) == %s", operation, example.Arguments, example.Result))] = true
		}
	}
	return restated
}

func significant(line string) bool {
	return len(line) >= MinLeakLineLength && !strings.HasPrefix(line, "import ") && !strings.HasPrefix(line, "from ")
}

// squeeze removes every space so that formatting differences do not hide a line.
func squeeze(line string) string { return strings.Join(strings.Fields(line), "") }

func lineSet(text string) map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		set[strings.TrimSpace(line)] = true
	}
	return set
}
