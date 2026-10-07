package junit_test

import (
	"fmt"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/junit"
)

func sampleTests() []junit.Test {
	outer := junit.Class{Name: "CartTest", Line: 3}
	nested := junit.Class{Name: "Bulk", Display: tagged("bulk orders [cites PAR-3~1]"), Line: 20}
	return []junit.Test{
		{Method: "addsUp", Display: tagged("adds up [cites PAR-1~1]"), Classes: []junit.Class{outer}, StartLine: 5, EndLine: 8},
		{Method: "plain", Classes: []junit.Class{outer}, StartLine: 10, EndLine: 12},
		{Method: "defaults", Kind: junit.Parameterized, Classes: []junit.Class{outer}, StartLine: 14, EndLine: 16},
		{Method: "named", Kind: junit.Parameterized, Template: tagged("{0} costs {1} [cites PAR-2~1]"), Classes: []junit.Class{outer}, StartLine: 18, EndLine: 19},
		{Method: "plain", Classes: []junit.Class{outer, nested}, StartLine: 22, EndLine: 24},
		{Method: "again", Kind: junit.Repeated, Classes: []junit.Class{outer}, StartLine: 26, EndLine: 27},
	}
}

func TestLocate(t *testing.T) {
	tests := sampleTests()
	cases := []struct {
		name, classname string
		line            int
		want            int
	}{
		{tagged("adds up [cites PAR-1~1]"), "CartTest", 0, 0},
		{"adds up", "shop.CartTest", 0, 0},
		{"addsUp()", "shop.CartTest", 0, 0},
		{"addsUp(int)[2]", "shop.CartTest", 0, 0},
		{tagged("7 costs 9 [cites PAR-2~1]"), "shop.CartTest", 0, 3},
		{"[3] a, b", "shop.CartTest", 0, 2},
		{"repetition 2 of 5", "shop.CartTest", 0, 5},
		{"plain()", "shop.CartTest", 0, 1},
		{"plain()", "shop.CartTest$Bulk", 0, 4},
		{"plain", tagged("CartTest bulk orders [cites PAR-3~1]"), 0, 4},
		{"plain", "CartTest", 0, 1},
	}
	for index, c := range cases {
		t.Run(fmt.Sprintf("report %d names the test at %d [verifies SP-1212~1]", index, c.want), func(t *testing.T) {
			got, found := junit.Locate(tests, c.name, c.classname, c.line)
			if !found || got != c.want {
				t.Fatalf("got %d (found %v), want %d", got, found, c.want)
			}
		})
	}

	t.Run("two tests that no name or class tells apart are told apart by the line [verifies SP-1212~1]", func(t *testing.T) {
		twins := []junit.Test{
			{Method: "same", Classes: []junit.Class{{Name: "A", Line: 1}}, StartLine: 3, EndLine: 5},
			{Method: "same", Classes: []junit.Class{{Name: "A", Line: 1}}, StartLine: 7, EndLine: 9},
		}
		if got, _ := junit.Locate(twins, "same()", "A", 8); got != 1 {
			t.Errorf("line 8: %d", got)
		}
		if got, _ := junit.Locate(twins, "same()", "A", 4); got != 0 {
			t.Errorf("line 4: %d", got)
		}
	})

	t.Run("a name that no test answers to is not found [verifies SP-1212~1]", func(t *testing.T) {
		if _, found := junit.Locate(tests, "nothing", "CartTest", 0); found {
			t.Fatal("found")
		}
		if _, found := junit.Locate(nil, "x", "", 0); found {
			t.Fatal("found in no tests")
		}
	})
}

func TestWords(t *testing.T) {
	t.Run("the words of a test are its method, its display names and its pattern without citations or placeholders [verifies SP-1212~1]", func(t *testing.T) {
		test := junit.Test{
			Method: "remoteCostsMore", Display: tagged("costs more [cites PAR-2~1]"), Template: tagged("{0} kilos [cites PAR-2~1]"),
			Classes: []junit.Class{{Name: "PricingTest"}, {Name: "Nested", Display: "with lines"}},
		}
		want := []string{"remoteCostsMore", "costs more", "kilos", "PricingTest", "Nested", "with lines"}
		got := test.Words()
		if len(got) != len(want) {
			t.Fatalf("got %q, want %q", got, want)
		}
		for index := range want {
			if got[index] != want[index] {
				t.Errorf("word %d: got %q, want %q", index, got[index], want[index])
			}
		}
	})
}
