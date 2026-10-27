package accept

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	boxofficeFixture = "../../../../../demo/loop/fixture/boxoffice"
	boxofficeSource  = "../../../internal/demoloop/fixturegate/testdata/task/boxoffice"
)

// oracleSample is the kind of file an independent test writer produces for the
// cinema spec: imports, the inline example, the boundaries and an enumeration.
const oracleSample = `import pytest

from boxoffice.prices import ticket_price


def test_example():
    assert ticket_price(age=2) == 0


def test_free_entry_below_three():
    for age in range(0, 3):
        assert ticket_price(age=age) == 0


@pytest.mark.parametrize("age", [3, 4, 11])
def test_children_pay_six(age):
    assert ticket_price(age=age) == 6
`

func boxofficeRound(t *testing.T, oracle string) *round {
	t.Helper()
	fixture, err := readTree(boxofficeFixture)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := readTree(boxofficeSource)
	if err != nil {
		t.Fatal(err)
	}
	m := &material{
		fixture: fixture, reference: reference, oracle: tree{"test_box_4.py": []byte(oracle)},
		task: study.Task{
			Name: "boxoffice", Spec: "specs/tickets.md", Source: "boxoffice/prices.py", Criterion: "BOX-4",
			Interface: study.Interface{Module: "boxoffice.prices", Operation: "ticket_price", Signature: "ticket_price(age)"},
			Truth:     study.Truth{Reference: "tools/x/reference", Violations: "experiments/loop-1/tasks/boxoffice/violations", Refactors: "experiments/loop-1/tasks/boxoffice/refactors", Oracle: "experiments/loop-1/tasks/boxoffice/oracle"},
		},
	}
	edited := reference["prices.py"]
	m.violations = []variant{{Criterion: "BOX-1", Number: "1", Edits: tree{"prices.py": []byte(strings.Replace(string(edited), "SENIOR_FROM_AGE = 65", "SENIOR_FROM_AGE = 66", 1))}}}
	return &round{m: m, criteria: []string{"BOX-1", "BOX-2", "BOX-3", "BOX-4"}}
}

func TestLeakOnTheCommittedFixture(t *testing.T) {
	t.Run("the cinema fixture and an independent oracle do not leak, whatever imports and inline examples they share [verifies SP-2556~1]", func(t *testing.T) {
		r := boxofficeRound(t, oracleSample)
		result, err := r.leak(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !result.Passed {
			t.Fatalf("A8 fails on the committed fixture: %+v", result.Findings)
		}
	})

	t.Run("an identical assertion line is a leak, reported to the author and, by position only, to the test writer [verifies SP-2556~1]", func(t *testing.T) {
		oracle := oracleSample + "\n\ndef test_adults_pay_twelve():\n    assert ticket_price(age=12) == 12\n"
		result, err := boxofficeRound(t, oracle).leak(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if result.Passed {
			t.Fatal("A8 passed")
		}
		var author, oracleText string
		for _, finding := range result.Findings {
			if finding.Role == records.RoleAuthor {
				author += finding.Text
			} else {
				oracleText += finding.Text
			}
		}
		if !strings.Contains(author, "tests/test_prices.py") || strings.Contains(oracleText, "ticket_price") || !strings.Contains(oracleText, "test_box_4.py") {
			t.Fatalf("author %q oracle %q", author, oracleText)
		}
	})

	t.Run("a line that a violation adds and a fixture file holds is a leak, a line it shares with the reference is not [verifies SP-2556~1]", func(t *testing.T) {
		r := boxofficeRound(t, oracleSample)
		r.m.fixture["tests/test_prices.py"] = append(r.m.fixture["tests/test_prices.py"], []byte("\nSENIOR_FROM_AGE = 66\n")...)
		result, err := r.leak(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if result.Passed || !strings.Contains(result.Findings[0].Text, "SENIOR_FROM_AGE = 66") {
			t.Fatalf("%+v", result.Findings)
		}
		if len(result.Findings) != 1 {
			t.Fatalf("lines shared with the reference were reported: %+v", result.Findings)
		}
	})
}

func TestLeakWordsAndPaths(t *testing.T) {
	cases := map[string]string{
		"the word oracle":     "# the oracle decides\n",
		"the word Mutants":    "Mutants are small edits\n",
		"the word planted":    "a planted defect\n",
		"the word weakly":     "weakly checked\n",
		"the word deliberate": "deliberately wrong\n",
		"a truth path":        "see experiments/loop-1/tasks/boxoffice/violations\n",
	}
	for name, line := range cases {
		t.Run(name+" is a leak [verifies SP-2556~1]", func(t *testing.T) {
			r := boxofficeRound(t, oracleSample)
			r.m.fixture["README.txt"] = []byte(line)
			result, err := r.leak(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if result.Passed {
				t.Fatal("A8 passed")
			}
		})
	}
}

func TestFixtureFilesAreReadWithoutTheirCaches(t *testing.T) {
	t.Run("a tree skips the caches a run leaves behind [verifies SP-2551~1]", func(t *testing.T) {
		files, err := readTree(filepath.Clean(boxofficeFixture))
		if err != nil {
			t.Fatal(err)
		}
		for name := range files {
			if strings.Contains(name, "__pycache__") {
				t.Errorf("%s", name)
			}
		}
	})
}
