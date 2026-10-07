package golang

import (
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/masking"
)

const (
	minNameWords  = 1
	maxNameWords  = 8
	maxIDNumber   = 9999
	maxRevision   = 3
	maxExtraRefs  = 3
	subtestAnchor = "another"
)

var nameVocabulary = []string{
	"amber", "birch", "cedar", "delta", "ember", "fjord", "glint", "harbor", "indigo", "juniper",
}

type testShape int

const (
	shapeTopLevel testShape = iota
	shapeLiteralSubtests
	shapeTableDriven
)

var testShapes = []testShape{shapeTopLevel, shapeLiteralSubtests, shapeTableDriven}

// generatedTest is a random test file with the bound test it holds.
type generatedTest struct {
	source string
	name   string
	refs   []criterion.Ref
}

func drawWords(t *rapid.T, label string) []string {
	return rapid.SliceOfN(rapid.SampledFrom(nameVocabulary), minNameWords, maxNameWords).Draw(t, label)
}

func drawRef(t *rapid.T, label string) criterion.Ref {
	return criterion.Ref{
		ID:       fmt.Sprintf("SP-%d", rapid.IntRange(1, maxIDNumber).Draw(t, label+"-id")),
		Revision: rapid.IntRange(1, maxRevision).Draw(t, label+"-revision"),
	}
}

func capitalized(word string) string { return strings.ToUpper(word[:1]) + word[1:] }

func camelCase(words []string) string {
	var joined strings.Builder
	for _, word := range words {
		joined.WriteString(capitalized(word))
	}
	return joined.String()
}

func drawTest(t *rapid.T) generatedTest {
	topWords, subWords := drawWords(t, "top"), drawWords(t, "sub")
	cited := drawRef(t, "cited")
	refs := []criterion.Ref{cited}
	for index := range rapid.IntRange(0, maxExtraRefs).Draw(t, "extra") {
		refs = append(refs, drawRef(t, fmt.Sprintf("extra%d", index)))
	}
	shape := rapid.SampledFrom(testShapes).Draw(t, "shape")
	return buildTest(shape, topWords, subWords, cited, refs)
}

func buildTest(shape testShape, topWords, subWords []string, cited criterion.Ref, refs []criterion.Ref) generatedTest {
	top := "Test" + camelCase(topWords)
	title := strings.Join(subWords, " ")
	citation := fmt.Sprintf("[verifies %s~%d]", cited.ID, cited.Revision)
	helper := "check" + camelCase(subWords)
	preamble := fmt.Sprintf("package parcel\n\nimport \"testing\"\n\n// %s verifies %s: %s\n", top, cited.ID, title)
	helperSource := fmt.Sprintf("\n// %s compares a weight.\nfunc %s(t *testing.T, got int) {\n\tif got != 3 {\n\t\tt.Fatalf(\"got %%d\", got)\n\t}\n}\n", helper, helper)
	name, body := top, ""
	switch shape {
	case shapeTopLevel:
		body = fmt.Sprintf("\t%s(t, Weigh(1))\n", helper)
	case shapeLiteralSubtests:
		name = top + "/" + strings.ReplaceAll(title+" "+citation, " ", "_")
		body = fmt.Sprintf("\tt.Run(%q, func(t *testing.T) { %s(t, Weigh(1)) })\n\tt.Run(%q, func(t *testing.T) { %s(t, Weigh(2)) })\n",
			title+" "+citation, helper, subtestAnchor+" "+title, helper)
	case shapeTableDriven:
		name = top + "/" + strings.ReplaceAll(title+" "+citation, " ", "_")
		body = fmt.Sprintf("\tfor _, title := range []string{%q, %q} {\n\t\tt.Run(title+%q, func(t *testing.T) { %s(t, Weigh(1)) })\n\t}\n",
			title, subtestAnchor, " "+citation, helper)
	}
	source := preamble + fmt.Sprintf("func %s(t *testing.T) {\n%s}\n", top, body) + helperSource
	return generatedTest{source: source, name: name, refs: refs}
}

func TestNoLeakageForAnyTestName(t *testing.T) {
	t.Run("no run of the test name, id or marker survives masking [verifies SP-325~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			generated := drawTest(t)
			masked, err := New().Mask(moduleProject(generated.source), bound(internalFile, generated.name))
			if err != nil {
				t.Fatalf("mask %s: %v\n%s", generated.name, err, generated.source)
			}
			for label, item := range map[string]string{"unit": masked.Unit, "signatures": masked.Signatures} {
				if leaks := masking.Check(item, generated.name, generated.refs); len(leaks) > 0 {
					t.Fatalf("%s leaks %v for %s\n%s", label, leaks, generated.name, item)
				}
			}
			assertParses(t, masked.Unit)
		})
	})
}

func TestNoLeakageInTheFixture(t *testing.T) {
	refs := []criterion.Ref{{ID: "PRC-12", Revision: 1}, {ID: "PRC-13", Revision: 1}, {ID: "PRC-14", Revision: 1}}
	t.Run("every fixture test masks without a leak [verifies SP-325~1]", func(t *testing.T) {
		for _, c := range goldenCases {
			masked := maskFixture(t, c.file, c.name)
			for label, item := range map[string]string{"unit": masked.Unit, "signatures": masked.Signatures} {
				if leaks := masking.Check(item, c.name, refs); len(leaks) > 0 {
					t.Errorf("%s of %s leaks: %v", label, c.name, leaks)
				}
			}
		}
	})
	t.Run("a table row named like the bound subtest is neutralized [verifies SP-325~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, tablePriceTest).Unit
		if runs := masking.NameRuns(unit, tablePriceTest); len(runs) > 0 {
			t.Errorf("the unit still holds the runs %v", runs)
		}
	})
	t.Run("a marker with another case is removed too [verifies SP-325~1]", func(t *testing.T) {
		source := "package parcel\n\nfunc TestMarker() { println(\"x [Verifies PRC-1~1]\") }\n"
		unit := maskProject(t, moduleProject(source), internalFile, "TestMarker").Unit
		if leaks := masking.CheckText(unit, refs); len(leaks) > 0 {
			t.Errorf("leaks: %v\n%s", leaks, unit)
		}
	})
}
