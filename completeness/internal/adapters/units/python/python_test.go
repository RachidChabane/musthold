package python_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/completeness/internal/adapters/units/python"
	"github.com/RachidChabane/musthold/completeness/internal/domain/code"
	"github.com/RachidChabane/musthold/completeness/internal/domain/hunk"
	"github.com/RachidChabane/musthold/completeness/internal/testkit"
)

func fixture(t *testing.T, side, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(testkit.FixturesDir(), testkit.PythonLedger, side, "src", "invoicing", name))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func index(t *testing.T, file string, source string) code.File {
	t.Helper()
	indexed, err := python.New().Index(file, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return indexed
}

func qualnames(units []code.Unit) []string {
	names := make([]string, len(units))
	for position, unit := range units {
		names[position] = unit.Qualname
	}
	return names
}

const cart = `from decimal import Decimal

RATE = Decimal("0.2")
LIMIT: int = 10
first, second = 1, 2


def total(lines):
    """Add the lines."""
    return sum(lines)


class Cart:
    DEFAULT = 3

    def __init__(self, items):
        self.items = items

    @property
    def size(self):
        return len(self.items)

    class Line:
        def amount(self):
            return 1


@decorated
def decorated_total(lines):
    return total(lines)


def total(lines):
    return 0
`

func TestIndex(t *testing.T) {
	t.Run("functions, methods, decorated definitions and module-level assignments are units, in source order [verifies SP-505~1]", func(t *testing.T) {
		indexed := index(t, "cart.py", cart)
		want := []string{"RATE", "LIMIT", "total", "Cart.__init__", "Cart.size", "Cart.Line.amount", "decorated_total", "total#2"}
		if got := qualnames(indexed.Units); !slices.Equal(got, want) {
			t.Fatalf("units %v, want %v", got, want)
		}
		method := indexed.Units[4]
		if method.Name != "size" || method.Owner != "Cart" || method.Kind != code.KindFunction || method.StartLine != 19 {
			t.Fatalf("method %+v (its decorator is part of it)", method)
		}
		if nested := indexed.Units[5]; nested.Owner != "Line" {
			t.Fatalf("nested method %+v", nested)
		}
		if constant := indexed.Units[0]; constant.Kind != code.KindAssignment || constant.Owner != "" {
			t.Fatalf("assignment %+v", constant)
		}
	})

	t.Run("a unit mentions every name it uses, calls, attributes and parameters alike, sorted and without repeats [verifies SP-505~1]", func(t *testing.T) {
		indexed := index(t, "cart.py", cart)
		var size, decorated code.Unit
		for _, unit := range indexed.Units {
			switch unit.Qualname {
			case "Cart.size":
				size = unit
			case "decorated_total":
				decorated = unit
			}
		}
		for _, want := range []string{"len", "items", "self", "property"} {
			if !slices.Contains(size.Mentions, want) {
				t.Errorf("Cart.size does not mention %s: %v", want, size.Mentions)
			}
		}
		if !slices.Contains(decorated.Mentions, "total") || !slices.Contains(decorated.Mentions, "decorated") || !slices.Contains(decorated.Mentions, "lines") {
			t.Errorf("decorated_total mentions %v", decorated.Mentions)
		}
		if !slices.IsSorted(size.Mentions) || len(slices.Compact(slices.Clone(size.Mentions))) != len(size.Mentions) {
			t.Fatalf("mentions %v", size.Mentions)
		}
	})

	t.Run("a test file is told from a source file [verifies SP-505~1]", func(t *testing.T) {
		indexer := python.New()
		for _, test := range []string{"tests/test_totals.py", "pkg/test_totals.py", "pkg/totals_test.py", "pkg/conftest.py", "src/tests/helpers.py"} {
			if !indexer.IsTest(test) {
				t.Errorf("%s is not told as a test", test)
			}
		}
		for _, source := range []string{"src/invoicing/totals.py", "src/latest.py", "pkg/contest.py"} {
			if indexer.IsTest(source) {
				t.Errorf("%s is told as a test", source)
			}
		}
		if !indexer.Handles("a.py") || indexer.Handles("a.go") {
			t.Fatal("Handles disagrees with the extension")
		}
	})
}

func TestHunks(t *testing.T) {
	before := index(t, "totals.py", string(fixture(t, "base", "totals.py")))
	after := index(t, "totals.py", string(fixture(t, "change", "totals.py")))

	t.Run("the change of the ledger gives one hunk per new or changed unit and none for a docstring [verifies SP-505~1]", func(t *testing.T) {
		var changed []string
		for _, found := range hunk.Between(before.Units, after.Units) {
			changed = append(changed, found.Unit.Qualname)
		}
		want := []string{"TAX_RATE", "line_amount", "invoice_total", "apply_discount"}
		if !slices.Equal(changed, want) {
			t.Fatalf("hunks %v, want %v (format_total only gained a docstring)", changed, want)
		}
	})

	t.Run("a change of comments, blank lines and spacing alone is no hunk [verifies SP-505~1]", func(t *testing.T) {
		original := "def total(lines):\n    return sum(lines)\n\nRATE = 2\n"
		reformatted := "# totals\n\n\ndef total( lines ):\n    # add them\n    return sum( lines )   # done\n\n\nRATE   =   2  # percent\n"
		if got := hunk.Between(index(t, "a.py", original).Units, index(t, "a.py", reformatted).Units); len(got) != 0 {
			t.Fatalf("hunks %+v", got)
		}
	})

	t.Run("a change of the body, of a literal or of a decorator is a hunk [verifies SP-505~1]", func(t *testing.T) {
		original := index(t, "a.py", "@cache\ndef total(lines):\n    return sum(lines)\n\nRATE = 2\n").Units
		for name, changed := range map[string]string{
			"body":      "@cache\ndef total(lines):\n    return max(lines)\n\nRATE = 2\n",
			"literal":   "@cache\ndef total(lines):\n    return sum(lines)\n\nRATE = 3\n",
			"decorator": "@lru\ndef total(lines):\n    return sum(lines)\n\nRATE = 2\n",
			"nesting":   "@cache\ndef total(lines):\n    if lines:\n        return sum(lines)\n\nRATE = 2\n",
		} {
			if got := hunk.Between(original, index(t, "a.py", changed).Units); len(got) == 0 {
				t.Errorf("%s: no hunk", name)
			}
		}
	})
}
