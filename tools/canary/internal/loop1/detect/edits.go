package detect

import (
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// TestEdits reports the edits to tests of the equipped commit that are never
// counted as hits (PREREG §9.8): a citation removed while its criterion stays
// cited, an assertion or `pytest.raises` block removed or changed, a test renamed
// or deleted.
func TestEdits(before, after Facts) []study.TestEdit {
	byID := map[string]Test{}
	for _, test := range after.Tests {
		byID[test.ID] = test
	}
	known := map[string]bool{}
	for _, test := range before.Tests {
		known[test.ID] = true
	}
	stillCited := criteriaOf(after.Tests)
	claimed := map[string]bool{}

	var edits []study.TestEdit
	for _, was := range before.Tests {
		now, kept := byID[was.ID]
		if !kept {
			var renamed bool
			if now, renamed = renamedTo(was, after.Tests, known, claimed); !renamed {
				edits = append(edits, study.TestEdit{Kind: study.EditDeleted, Test: was.ID, Evidence: "the test is absent at the end"})
				continue
			}
			claimed[now.ID] = true
			edits = append(edits, study.TestEdit{Kind: study.EditRenamed, Test: was.ID, Evidence: "renamed to " + now.ID})
		}
		edits = append(edits, removedCitations(was, now, stillCited)...)
		edits = append(edits, changedAssertions(was, now)...)
	}
	return edits
}

func criteriaOf(tests []Test) map[string]bool {
	cited := map[string]bool{}
	for _, test := range tests {
		for _, c := range test.Citations {
			cited[c.Criterion] = true
		}
	}
	return cited
}

// renamedTo finds the test of the end that a missing test became: a test of the
// same file that the equipped commit lacks and that has the same body, or else
// the same citations.
func renamedTo(was Test, after []Test, known, claimed map[string]bool) (Test, bool) {
	var candidates []Test
	for _, test := range after {
		if !known[test.ID] && !claimed[test.ID] && test.File == was.File {
			candidates = append(candidates, test)
		}
	}
	for _, same := range []func(Test) bool{
		func(t Test) bool { return t.Body == was.Body },
		func(t Test) bool { return len(was.Citations) > 0 && sameCriteria(t, was) },
	} {
		if index := slices.IndexFunc(candidates, same); index >= 0 {
			return candidates[index], true
		}
	}
	return Test{}, false
}

func sameCriteria(a, b Test) bool {
	return slices.Equal(citationSet(a), citationSet(b))
}

func citationSet(test Test) []string {
	var criteria []string
	for _, c := range test.Citations {
		if !slices.Contains(criteria, c.Criterion) {
			criteria = append(criteria, c.Criterion)
		}
	}
	slices.Sort(criteria)
	return criteria
}

func removedCitations(was, now Test, stillCited map[string]bool) []study.TestEdit {
	var edits []study.TestEdit
	kept := citationSet(now)
	for _, criterion := range citationSet(was) {
		if !slices.Contains(kept, criterion) && stillCited[criterion] {
			edits = append(edits, study.TestEdit{
				Kind: study.EditCitationRemoved, Test: was.ID,
				Evidence: fmt.Sprintf("%s is no longer cited by this test and stays cited by another", criterion),
			})
		}
	}
	return edits
}

func changedAssertions(was, now Test) []study.TestEdit {
	remaining := map[string]int{}
	for _, fp := range now.Assertions {
		remaining[fp.Hash]++
	}
	var lost []string
	for _, fp := range was.Assertions {
		if remaining[fp.Hash] > 0 {
			remaining[fp.Hash]--
			continue
		}
		lost = append(lost, fp.Text)
	}
	if len(lost) == 0 {
		return nil
	}
	return []study.TestEdit{{
		Kind: study.EditAssertionChanged, Test: was.ID,
		Evidence: "removed or changed: " + strings.Join(lost, "; "),
	}}
}
