package detect

import (
	"fmt"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// TestsDir is the directory whose files hold the tests the proofs read.
const TestsDir = "tests/"

// citedBy maps each criterion that a test under tests/ cites to its first citing
// test, in the order of the facts.
func citedBy(facts Facts) map[string]citation {
	cited := map[string]citation{}
	for _, test := range facts.Tests {
		if !strings.HasPrefix(test.File, TestsDir) {
			continue
		}
		for _, c := range test.Citations {
			if _, seen := cited[c.Criterion]; !seen {
				cited[c.Criterion] = citation{test: test.ID, file: test.File, line: c.Line}
			}
		}
	}
	return cited
}

type citation struct {
	test string
	file string
	line int
}

// Uncited is detector K1: a criterion that a test under tests/ cites at the
// equipped commit is cited by no test at the end.
func Uncited(before, after Facts) []study.Hit {
	stillCited := citedBy(after)
	origins := citedBy(before)
	var hits []study.Hit
	for _, criterion := range orderedCriteria(before, origins) {
		if _, kept := stillCited[criterion]; kept {
			continue
		}
		origin := origins[criterion]
		hits = append(hits, study.Hit{
			Detector: study.DetectorUncited,
			File:     origin.file,
			Line:     origin.line,
			Evidence: fmt.Sprintf("%s is cited by %s at the equipped commit and by no test at the end", criterion, origin.test),
		})
	}
	return hits
}

// orderedCriteria lists the cited criteria in the order their first citation
// appears.
func orderedCriteria(facts Facts, cited map[string]citation) []string {
	var ordered []string
	seen := map[string]bool{}
	for _, test := range facts.Tests {
		for _, c := range test.Citations {
			if _, isCited := cited[c.Criterion]; isCited && !seen[c.Criterion] {
				seen[c.Criterion] = true
				ordered = append(ordered, c.Criterion)
			}
		}
	}
	return ordered
}
