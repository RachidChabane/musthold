package testlabel_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/testlabel"
)

// marker is the word that opens a citation; the tests write it as `cites` so that
// none of their lines reads as a requirement tag.
const marker = "verif" + "ies"

func tagged(text string) string { return strings.ReplaceAll(text, "cites", marker) }

func TestCompact(t *testing.T) {
	t.Run("the citations and the separators of a reporter are taken out of a label [verifies SP-1314~1]", func(t *testing.T) {
		cases := map[string]string{
			tagged("line totals > multiply [cites INV-1~1]"):   "line totals multiply",
			tagged("a [cites INV-1~1] b [ cites  INV-2~1 ] c"): "a b c",
			"has [brackets] but no citation":                   "has [brackets] but no citation",
			"  spaced   out  ":                                 "spaced out",
		}
		for label, want := range cases {
			if got := testlabel.Compact(label); got != want {
				t.Errorf("%q: got %q, want %q", label, got, want)
			}
		}
	})
}

func TestPatterns(t *testing.T) {
	placeholders := regexp.MustCompile(`%[sd]`)

	t.Run("a title matches the labels it expands to, in either reporter, and no others [verifies SP-1314~1]", func(t *testing.T) {
		patterns := []string{testlabel.PatternOf(tagged("take %s percent off [cites INV-1~1]"), placeholders)}
		for _, label := range []string{
			tagged("discounts take 10 percent off [cites INV-1~1]"),
			tagged("discounts > take 10 percent off [cites INV-1~1]"),
			tagged("test/cart.test.ts.discounts > take x y percent off [cites INV-1~1]"),
			tagged("take 5 percent off [cites INV-1~1].take 5 percent off [cites INV-1~1]"),
		} {
			if !testlabel.Matches(patterns, label) {
				t.Errorf("%q does not match", label)
			}
		}
		for _, label := range []string{"give 10 percent off", "take 10 percent", "take 10 percentage off"} {
			if testlabel.Matches(patterns, label) {
				t.Errorf("%q matches", label)
			}
		}
	})

	t.Run("the characters a pattern reads specially match themselves in a title, and a pattern that is invalid matches nothing [verifies SP-1314~1]", func(t *testing.T) {
		if !testlabel.Matches([]string{testlabel.PatternOf("costs (a+b) [x]? $5", placeholders)}, "costs (a+b) [x]? $5") {
			t.Error("the title was read as a pattern")
		}
		if testlabel.Matches([]string{"(unclosed"}, "anything") {
			t.Error("an invalid pattern matched")
		}
		if testlabel.Matches(nil, "anything") {
			t.Error("no pattern matched")
		}
	})
}

func TestJUnitPattern(t *testing.T) {
	outer := testlabel.JUnitClass{Name: "TotalsTest"}
	nested := testlabel.JUnitClass{Name: "Invoice", Display: tagged("an invoice [cites INV-1~1]")}

	match := func(test testlabel.JUnitTest, label string) bool {
		return testlabel.Matches([]string{testlabel.JUnitPattern(test)}, tagged(label))
	}

	t.Run("a method is reported by its name, with or without its signature and the index of an invocation [verifies SP-1314~1]", func(t *testing.T) {
		test := testlabel.JUnitTest{Method: "addsTheLines", Classes: []testlabel.JUnitClass{outer}}
		for _, label := range []string{"ledger.TotalsTest.addsTheLines", "ledger.TotalsTest.addsTheLines()", "ledger.TotalsTest.addsTheLines(int)[2]"} {
			if !match(test, label) {
				t.Errorf("%q does not match", label)
			}
		}
		for _, label := range []string{"ledger.TotalsTest.addsTheLinesAgain", "ledger.OtherTest.addsTheLines", "ledger.TotalsTest.other"} {
			if match(test, label) {
				t.Errorf("%q matches", label)
			}
		}
	})

	t.Run("a display name is reported with its citation, and a pattern with its placeholders expanded [verifies SP-1314~1]", func(t *testing.T) {
		displayed := testlabel.JUnitTest{Method: "adds", Display: tagged("adds the lines [cites INV-1~1]"), Classes: []testlabel.JUnitClass{outer}}
		if !match(displayed, "ledger.TotalsTest.adds the lines [cites INV-1~1]") {
			t.Error("the display name does not match")
		}
		named := testlabel.JUnitTest{Method: "costs", Parameterized: true, Template: tagged("{0} kilos cost {1} [cites INV-2~1]"), Classes: []testlabel.JUnitClass{outer}}
		if !match(named, "ledger.TotalsTest.7 kilos cost 9 [cites INV-2~1]") || match(named, "ledger.TotalsTest.7 kilos") {
			t.Error("the name pattern does not match what it expands to")
		}
	})

	t.Run("the default name of an invocation or a repetition is reported when the author gave none [verifies SP-1314~1]", func(t *testing.T) {
		parametrized := testlabel.JUnitTest{Method: "costs", Parameterized: true, Classes: []testlabel.JUnitClass{outer}}
		repeated := testlabel.JUnitTest{Method: "again", Repeated: true, Classes: []testlabel.JUnitClass{outer}}
		if !match(parametrized, "ledger.TotalsTest.[3] 10, 200") || match(repeated, "ledger.TotalsTest.[3] 10, 200") {
			t.Error("an invocation is not matched as a parametrized test only")
		}
		if !match(repeated, "ledger.TotalsTest.repetition 2 of 5") || match(parametrized, "ledger.TotalsTest.repetition 2 of 5") {
			t.Error("a repetition is not matched as a repeated test only")
		}
	})

	t.Run("a nested class is reported as a binary name or by its display name [verifies SP-1314~1]", func(t *testing.T) {
		test := testlabel.JUnitTest{Method: "adds", Classes: []testlabel.JUnitClass{outer, nested}}
		if !match(test, "ledger.TotalsTest$Invoice.adds") || !match(test, "TotalsTest an invoice [cites INV-1~1].adds") {
			t.Error("the nested class is not matched")
		}
		if match(test, "ledger.TotalsTest.adds") {
			t.Error("the enclosing class alone is matched")
		}
	})
}
