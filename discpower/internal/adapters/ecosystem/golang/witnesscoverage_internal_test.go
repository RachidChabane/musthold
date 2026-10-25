package golang

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
)

const verifiesWitnessUnits = " [verifies SP-2270~1]"

// stubLocator locates the same definitions in every file.
type stubLocator struct {
	definitions []ecosystem.Definition
}

func (l stubLocator) Locate(string, []byte) ([]ecosystem.Definition, error) {
	return l.definitions, nil
}

func TestInnermost(t *testing.T) {
	outer := ecosystem.Definition{Qualname: "tally.Outer", StartLine: 3, EndLine: 12}
	inner := ecosystem.Definition{Qualname: "tally.Outer.func1", StartLine: 5, EndLine: 7}
	definitions := []ecosystem.Definition{outer, inner}
	for _, c := range []struct {
		name  string
		line  int
		want  string
		found bool
	}{
		{"a line of the inner definition is the inner one's", 6, inner.Qualname, true},
		{"a line of the outer definition alone is the outer one's", 10, outer.Qualname, true},
		{"a line outside every definition is no definition's", 1, "", false},
	} {
		t.Run(c.name+verifiesWitnessUnits, func(t *testing.T) {
			got, found := innermost(definitions, c.line)
			if found != c.found || got.Qualname != c.want {
				t.Fatalf("innermost(%d) = %q, %t", c.line, got.Qualname, found)
			}
		})
	}
}

func TestTestsByPackage(t *testing.T) {
	t.Run("tests group by the directory of their file, in the order each directory first appears"+verifiesWitnessUnits, func(t *testing.T) {
		root := ecosystem.NewBoundTest("example.com/tally", "Test_a", "zz_specproof_a_test.go", 3)
		nested := ecosystem.NewBoundTest("example.com/tally/rates", "Test_b", "rates/zz_specproof_b_test.go", 3)
		again := ecosystem.NewBoundTest("example.com/tally", "Test_c", "zz_specproof_a_test.go", 9)
		groups := testsByPackage([]ecosystem.BoundTest{root, nested, again})
		if len(groups) != 2 || groups[0].dir != "." || groups[1].dir != "rates" ||
			!slices.Equal(groups[0].tests, []ecosystem.BoundTest{root, again}) || !slices.Equal(groups[1].tests, []ecosystem.BoundTest{nested}) {
			t.Fatalf("groups = %+v", groups)
		}
	})
}

func TestRanBeyondTheStartUp(t *testing.T) {
	t.Run("a block counts for a test only when the test ran it more often than the start-up did"+verifiesWitnessUnits, func(t *testing.T) {
		startUp := readBlockCounts([]byte("mode: atomic\nexample.com/tally/tally.go:3.20,5.2 1 1\nexample.com/tally/tally.go:7.20,9.2 1 0\n"))
		ran := readBlockCounts([]byte("mode: atomic\nexample.com/tally/tally.go:3.20,5.2 1 1\nexample.com/tally/tally.go:7.20,9.2 1 2\n"))
		blocks := ranBeyond(ran, startUp)
		if len(blocks) != 1 || blocks[0].first != 7 || blocks[0].last != 9 {
			t.Fatalf("blocks = %+v", blocks)
		}
	})
}

func TestUnitsOfBlocks(t *testing.T) {
	locator := stubLocator{definitions: []ecosystem.Definition{
		{Qualname: "tally.Discount", StartLine: 3, EndLine: 6},
		{Qualname: "tally.Shipping", StartLine: 8, EndLine: 12},
	}}
	t.Run("the blocks name each definition holding one of their lines once, sorted, leaving out files outside the module"+verifiesWitnessUnits, func(t *testing.T) {
		blocks := []profileBlock{
			{file: "example.com/tally/tally.go", first: 9, last: 10},
			{file: "example.com/tally/tally.go", first: 4, last: 5},
			{file: "example.com/tally/tally.go", first: 11, last: 11},
			{file: "example.com/other/other.go", first: 4, last: 4},
		}
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "tally.go"), []byte("package tally\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := newUnitIndex(root, locator).of(blocks, tallyModule)
		want := []footprint.Ref{{File: "tally.go", Qualname: "tally.Discount"}, {File: "tally.go", Qualname: "tally.Shipping"}}
		if !slices.Equal(got, want) {
			t.Fatalf("units = %v, want %v", got, want)
		}
	})
}
