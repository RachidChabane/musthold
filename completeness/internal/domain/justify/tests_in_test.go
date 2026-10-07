package justify_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/justify"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/reach"
)

// marker is the word that opens a citation; the tests write it as `cites` so that
// none of their lines reads as a requirement tag.
const marker = "verif" + "ies"

func tagged(text string) string { return strings.ReplaceAll(text, "cites", marker) }

func testUnit(qualname string, pattern string, mentions ...string) code.Unit {
	return code.Unit{File: "test/totals.test.ts", Qualname: qualname, Name: qualname, Kind: code.KindTest, Labels: []string{pattern}, Mentions: mentions}
}

func TestTestEntriesIn(t *testing.T) {
	multiplies := testUnit("amounts > multiply", `amounts\s+multiply`, "lineAmount", "expect")
	adds := testUnit("invoices > add", `invoices\s+add`, "invoiceTotal")
	units := []code.Unit{multiplies, adds}

	t.Run("a label designates the names that the test units it matches mention [verifies SP-1309~1]", func(t *testing.T) {
		got := justify.TestEntriesIn(tagged("test/totals.test.ts.amounts > multiply [cites INV-1~1]"), units)
		if !slices.Equal(got, []string{"expect", "lineAmount", "test"}) {
			t.Fatalf("entries %v", got)
		}
		got = justify.TestEntriesIn("invoices add", units)
		if !slices.Equal(got, []string{"invoiceTotal"}) {
			t.Fatalf("entries %v", got)
		}
	})

	t.Run("the function names a label holds are entries too, and a label no unit matches gives only those [verifies SP-1309~1]", func(t *testing.T) {
		got := justify.TestEntriesIn("tests.test_totals.test_line_amount", units)
		if !slices.Equal(got, []string{"test_line_amount", "test_totals", "tests"}) {
			t.Fatalf("entries %v", got)
		}
		if got := justify.TestEntriesIn("nothing alike", units); len(got) != 0 {
			t.Fatalf("entries %v", got)
		}
	})

	t.Run("a unit that is not a test is never designated by a label [verifies SP-1309~1]", func(t *testing.T) {
		function := code.Unit{File: "src/totals.ts", Qualname: "lineAmount", Name: "lineAmount", Kind: code.KindFunction, Labels: []string{`amounts`}, Mentions: []string{"price"}}
		if got := justify.TestEntriesIn("amounts", []code.Unit{function}); len(got) != 0 {
			t.Fatalf("entries %v", got)
		}
	})

	t.Run("what a matched test mentions is reached across the project [verifies SP-1309~1]", func(t *testing.T) {
		line := code.Unit{File: "src/totals.ts", Qualname: "lineAmount", Name: "lineAmount", Kind: code.KindFunction, Mentions: []string{"TAX"}}
		tax := code.Unit{File: "src/totals.ts", Qualname: "TAX", Name: "TAX", Kind: code.KindAssignment}
		other := code.Unit{File: "src/totals.ts", Qualname: "other", Name: "other", Kind: code.KindFunction}
		graph := reach.NewGraph([]code.Unit{line, tax, other})
		entries := justify.TestEntriesIn("amounts multiply", append([]code.Unit{line, tax, other}, multiplies))
		reached := graph.Reached(entries)
		if !reached[line.Key()] || !reached[tax.Key()] || reached[other.Key()] {
			t.Fatalf("reached %v from %v", reached, entries)
		}
	})
}

func TestOperationBindings(t *testing.T) {
	t.Run("a binding written as a Java method reference designates its method [verifies SP-1310~1]", func(t *testing.T) {
		cases := map[string]string{
			"ledger.Totals#invoiceTotal":  "invoiceTotal",
			"src/totals.ts:invoiceTotal":  "invoiceTotal",
			"ledger.Totals.invoiceTotal":  "invoiceTotal",
			"invoicing.totals:Cart.total": "total",
			"invoiceTotal":                "invoiceTotal",
		}
		for binding, want := range cases {
			if got := justify.OperationEntry(binding); got != want {
				t.Errorf("%q: %q, want %q", binding, got, want)
			}
		}
	})
}
