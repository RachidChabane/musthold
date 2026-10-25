package counterexample_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/counterexample"
)

const prices = `FREE = 0
SENIOR_FROM_AGE = 65
LIMITS = (
    1,
    2,
)


@cache
def ticket_price(age):
    if age >= SENIOR_FROM_AGE:
        return 8

    return 12


def other():
    return 1
`

func TestDecode(t *testing.T) {
	t.Run("the vault's edit set is read with its files in first-seen order [verifies SP-2404~1]", func(t *testing.T) {
		mutant, err := counterexample.Decode([]byte(`{"edits":[{"file":"a.py","qualname":"f","new_source":"def f():\n    pass"},{"file":"b.py","qualname":"X","target":"module_assignment","new_source":"X = 1"},{"file":"a.py","qualname":"g","new_source":"def g(): pass"}]}`))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(mutant.Files(), []string{"a.py", "b.py"}) || mutant.Edits[1].Target != "module_assignment" || mutant.Edits[0].NewSource != "def f():\n    pass" {
			t.Errorf("mutant = %+v", mutant)
		}
	})
}

func TestApply(t *testing.T) {
	t.Run("a definition is replaced with its decorators and body, the blank lines after it kept [verifies SP-2404~1]", func(t *testing.T) {
		mutant := counterexample.Mutant{Edits: []counterexample.Edit{{File: "p.py", Qualname: "ticket_price", NewSource: "def ticket_price(age):\n    return 12\n"}}}
		got, err := mutant.Apply("p.py", prices)
		if err != nil {
			t.Fatal(err)
		}
		want := "FREE = 0\nSENIOR_FROM_AGE = 65\nLIMITS = (\n    1,\n    2,\n)\n\n\ndef ticket_price(age):\n    return 12\n\n\ndef other():\n    return 1\n"
		if got != want {
			t.Errorf("got\n%s\nwant\n%s", got, want)
		}
	})

	t.Run("a module assignment is replaced with its continuation lines [verifies SP-2404~1]", func(t *testing.T) {
		for qualname, replaced := range map[string]string{
			"SENIOR_FROM_AGE": "FREE = 0\nSENIOR_FROM_AGE = 66\nLIMITS = (\n",
			"LIMITS":          "SENIOR_FROM_AGE = 65\nLIMITS = (1,)\n\n\n@cache",
		} {
			source := map[string]string{"SENIOR_FROM_AGE": "SENIOR_FROM_AGE = 66", "LIMITS": "LIMITS = (1,)"}[qualname]
			mutant := counterexample.Mutant{Edits: []counterexample.Edit{{File: "p.py", Qualname: qualname, Target: "module_assignment", NewSource: source}}}
			got, err := mutant.Apply("p.py", prices)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, replaced) {
				t.Errorf("%s: got\n%s", qualname, got)
			}
		}
	})

	t.Run("an edit of another file leaves the file as it is [verifies SP-2404~1]", func(t *testing.T) {
		mutant := counterexample.Mutant{Edits: []counterexample.Edit{{File: "q.py", Qualname: "other", NewSource: "def other(): pass"}}}
		if got, err := mutant.Apply("p.py", prices); err != nil || got != prices {
			t.Errorf("got %q, %v", got, err)
		}
	})

	t.Run("a unit that is not in the file, and a form the splicer does not place, are refused [verifies SP-2404~1]", func(t *testing.T) {
		for edit, want := range map[counterexample.Edit]error{
			{File: "p.py", Qualname: "missing", NewSource: "def missing(): pass"}:                  counterexample.ErrUnitNotFound,
			{File: "p.py", Qualname: "Klass.method", NewSource: "def method(self): pass"}:          counterexample.ErrUnsupported,
			{File: "p.py", Qualname: "other", Target: "statement", NewSource: "return 2"}:          counterexample.ErrUnsupported,
			{File: "p.py", Qualname: "FREE_X", Target: "module_assignment", NewSource: "FREE_X=1"}: counterexample.ErrUnitNotFound,
		} {
			if _, err := (counterexample.Mutant{Edits: []counterexample.Edit{edit}}).Apply("p.py", prices); !errors.Is(err, want) {
				t.Errorf("%+v: error %v, want %v", edit, err, want)
			}
		}
	})
}
