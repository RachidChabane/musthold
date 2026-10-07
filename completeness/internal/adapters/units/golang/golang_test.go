package golang_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/units/golang"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/hunk"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
)

func fixture(t *testing.T, side, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(testkit.FixturesDir(), testkit.GoLedger, side, name))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func index(t *testing.T, file string, source []byte) code.File {
	t.Helper()
	indexed, err := golang.New().Index(file, source)
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

func TestIndex(t *testing.T) {
	t.Run("functions, methods and package-level variables and constants are units, in source order [verifies SP-504~1]", func(t *testing.T) {
		source := `package cart

import "strings"

const rate = 2
var (
	label, suffix = "a", "b"
	_             = 3
)

type Cart struct{ lines []string }

func New() *Cart { return &Cart{} }

func (c *Cart) Add(line string) { c.lines = append(c.lines, strings.TrimSpace(line)) }

func (c Cart) Len() int { return len(c.lines) }

func init() {}

func init() {}
`
		indexed := index(t, "cart.go", []byte(source))
		want := []string{"rate", "label", "suffix", "New", "Cart.Add", "Cart.Len", "init", "init#2"}
		if got := qualnames(indexed.Units); !slices.Equal(got, want) {
			t.Fatalf("units %v, want %v", got, want)
		}
		method := indexed.Units[4]
		if method.Name != "Add" || method.Owner != "Cart" || method.Kind != code.KindFunction || method.StartLine != 15 {
			t.Fatalf("method %+v", method)
		}
		if constant := indexed.Units[0]; constant.Kind != code.KindAssignment || constant.Owner != "" {
			t.Fatalf("constant %+v", constant)
		}
	})

	t.Run("a unit mentions the identifiers it refers to, sorted and without repeats [verifies SP-504~1]", func(t *testing.T) {
		indexed := index(t, "totals.go", fixture(t, "base", "totals.go"))
		var total code.Unit
		for _, unit := range indexed.Units {
			if unit.Name == "InvoiceTotal" {
				total = unit
			}
		}
		for _, want := range []string{"LineAmount", "Line", "Price", "Quantity"} {
			if !slices.Contains(total.Mentions, want) {
				t.Errorf("InvoiceTotal does not mention %s: %v", want, total.Mentions)
			}
		}
		if !slices.IsSorted(total.Mentions) || len(slices.Compact(slices.Clone(total.Mentions))) != len(total.Mentions) {
			t.Fatalf("mentions %v", total.Mentions)
		}
	})

	t.Run("a test file is told from a source file and an other extension is not handled [verifies SP-504~1]", func(t *testing.T) {
		indexer := golang.New()
		if !indexer.Handles("pkg/totals.go") || indexer.Handles("totals.py") || indexer.Handles("README.md") {
			t.Fatal("Handles disagrees with the extension")
		}
		if !indexer.IsTest("pkg/totals_test.go") || indexer.IsTest("pkg/totals.go") {
			t.Fatal("IsTest disagrees with the suffix")
		}
		if !index(t, "totals_test.go", fixture(t, "base", "totals_test.go")).Test {
			t.Fatal("a test file is not marked as such")
		}
	})

	t.Run("a file that does not parse is an error [verifies SP-504~1]", func(t *testing.T) {
		if _, err := golang.New().Index("broken.go", []byte("package x\nfunc (")); err == nil {
			t.Fatal("indexed a file that does not parse")
		}
	})
}

func TestHunks(t *testing.T) {
	before := index(t, "totals.go", fixture(t, "base", "totals.go"))
	after := index(t, "totals.go", fixture(t, "change", "totals.go"))

	t.Run("the change of the ledger gives one hunk per new or changed unit and none for a comment [verifies SP-504~1]", func(t *testing.T) {
		var changed []string
		for _, found := range hunk.Between(before.Units, after.Units) {
			changed = append(changed, found.Unit.Qualname)
		}
		want := []string{"taxRate", "LineAmount", "InvoiceTotal", "ApplyDiscount"}
		if !slices.Equal(changed, want) {
			t.Fatalf("hunks %v, want %v (FormatTotal only gained a doc comment)", changed, want)
		}
	})

	t.Run("a change of formatting alone is no hunk [verifies SP-504~1]", func(t *testing.T) {
		reformatted := "package ledger\nimport \"fmt\"\nconst taxRate   =   0.20\ntype Line struct{ Price float64; Quantity int }\n" +
			"func LineAmount(price float64,quantity int) float64 { return price*float64(quantity) }\n" +
			"func InvoiceTotal(lines []Line) float64 {\n\ttotal := 0.0\n\tfor _, line := range lines { total += LineAmount(line.Price, line.Quantity) }\n\treturn total\n}\n" +
			"func FormatTotal(total float64) string { return fmt.Sprintf(\"%.2f EUR\", total) }\n"
		if got := hunk.Between(before.Units, index(t, "totals.go", []byte(reformatted)).Units); len(got) != 0 {
			t.Fatalf("hunks %+v", got)
		}
	})

	t.Run("a unit that exists in the base keeps its text before and a new one has none [verifies SP-504~1]", func(t *testing.T) {
		for _, found := range hunk.Between(before.Units, after.Units) {
			switch found.Unit.Qualname {
			case "ApplyDiscount":
				if !found.New || found.Before != "" {
					t.Errorf("ApplyDiscount %+v", found)
				}
			case "LineAmount":
				if found.New || found.Before == "" {
					t.Errorf("LineAmount %+v", found)
				}
			}
		}
	})
}
