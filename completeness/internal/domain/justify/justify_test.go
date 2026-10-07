package justify_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/justify"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/reach"
)

var (
	first  = criterion.Ref{ID: "INV-1", Revision: 1}
	second = criterion.Ref{ID: "INV-2", Revision: 1}
)

func unit(name string, mentions ...string) code.Unit {
	return code.Unit{File: "src/totals.py", Qualname: name, Name: name, Kind: code.KindFunction, Mentions: mentions}
}

func TestJustifiers(t *testing.T) {
	total := unit("invoice_total", "line_amount")
	line := unit("line_amount")
	discount := unit("apply_discount")
	graph := reach.NewGraph([]code.Unit{total, line, discount})

	t.Run("a unit that a probe or a test may execute is justified by the requirement they belong to [verifies SP-509~1]", func(t *testing.T) {
		got := justify.Justifiers(graph, []justify.Evidence{
			{Ref: first, Entries: []string{"invoice_total"}},
			{Ref: second, Entries: []string{"line_amount"}},
		})
		if !slices.Equal(got[total.Key()], []criterion.Ref{first}) {
			t.Fatalf("invoice_total is justified by %v", got[total.Key()])
		}
		if !slices.Equal(got[line.Key()], []criterion.Ref{first, second}) {
			t.Fatalf("line_amount is justified by %v", got[line.Key()])
		}
		if _, justified := got[discount.Key()]; justified {
			t.Fatal("a unit nothing executes is justified")
		}
	})

	t.Run("the requirements of a unit are sorted and without repeats [verifies SP-509~1]", func(t *testing.T) {
		got := justify.Justifiers(graph, []justify.Evidence{
			{Ref: second, Entries: []string{"line_amount"}},
			{Ref: first, Entries: []string{"line_amount"}},
			{Ref: first, Entries: []string{"line_amount"}},
		})
		if !slices.Equal(got[line.Key()], []criterion.Ref{first, second}) {
			t.Fatalf("justified by %v", got[line.Key()])
		}
	})

	t.Run("no evidence justifies nothing [verifies SP-509~1]", func(t *testing.T) {
		if got := justify.Justifiers(graph, nil); len(got) != 0 {
			t.Fatalf("justifiers %v", got)
		}
	})
}

func TestEntries(t *testing.T) {
	t.Run("a test label designates the test functions it names, for Python and for Go subtests [verifies SP-509~1]", func(t *testing.T) {
		cases := map[string][]string{
			"tests.test_totals.test_line_amount_multiplies":                  {"tests", "test_totals", "test_line_amount_multiplies"},
			"tests.test_totals.TestCart.test_total[3-4]":                     {"tests", "test_totals", "TestCart", "test_total"},
			"example.com/ledger.TestLineAmount/multiplies_price_by_quantity": {"TestLineAmount"},
			"example.com/ledger.TestLineAmount/case_1.5":                     {"TestLineAmount"},
			"example.com/ledger.ExampleTotal":                                {"ExampleTotal"},
			"plain":                                                          nil,
		}
		for label, want := range cases {
			if got := justify.TestEntries(label); !slices.Equal(got, want) {
				t.Errorf("%q: %v, want %v", label, got, want)
			}
		}
	})

	t.Run("an operation binding designates its function, whatever the module and class path [verifies SP-509~1]", func(t *testing.T) {
		cases := map[string]string{
			"invoicing.totals:invoice_total": "invoice_total",
			"invoicing:Cart.total":           "total",
			"ledger.InvoiceTotal":            "InvoiceTotal",
			"invoice_total":                  "invoice_total",
		}
		for binding, want := range cases {
			if got := justify.OperationEntry(binding); got != want {
				t.Errorf("%q: %q, want %q", binding, got, want)
			}
		}
	})
}
