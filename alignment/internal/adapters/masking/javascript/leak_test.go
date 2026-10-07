package javascript_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"unicode"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/masking/javascript"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/masking"
	"github.com/RachidChabane/spec-proof/alignment/internal/platform/testname"
)

// nameVocabulary holds words that no fixture of the property spells.
var nameVocabulary = []string{
	"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliet",
}

const (
	minNameWords = 3
	maxNameWords = 8
	maxCriteria  = 4
	shopLibrary  = "export class Cart {\n  readonly size: number;\n  constructor(size: number) { this.size = size; }\n}\n"
	rowValue     = "7"
)

type testShape int

const (
	plainTest testShape = iota
	tableTest
	tableSuiteTest
	shapeCount
)

func drawWords(t *rapid.T) []string {
	return rapid.SliceOfN(rapid.SampledFrom(nameVocabulary), minNameWords, maxNameWords).Draw(t, "words")
}

func drawRefs(t *rapid.T) []criterion.Ref {
	ids := rapid.SliceOfN(rapid.StringMatching(`[A-Z]{2,4}-[1-9][0-9]{0,3}`), 1, maxCriteria).Draw(t, "ids")
	refs := make([]criterion.Ref, len(ids))
	for index, id := range ids {
		refs[index] = criterion.Ref{ID: id, Revision: 1}
	}
	return refs
}

func camel(words []string) string {
	var out strings.Builder
	for _, word := range words {
		out.WriteString(string(unicode.ToUpper(rune(word[0]))) + word[1:])
	}
	return out.String()
}

// propertySource builds a test file around the words of a title: they appear in
// the titles with a citation, in a comment, in the names of a constant and of a
// helper function, and in a hook.
func propertySource(shape testShape, words []string, refs []criterion.Ref) string {
	title := strings.Join(words, " ")
	tag := fmt.Sprintf(tagged("[cites %s]"), refs[0])
	name := camel(words)
	header := []string{
		"import { describe, expect, it, beforeEach } from 'vitest';",
		"import { Cart } from '../src/shop';",
		"",
		"const limit" + name + " = 3;",
		"",
		"function make" + name + "() {",
		"  // " + title + " " + refs[0].ID,
		"  return new Cart(2);",
		"}",
		"",
	}
	var body []string
	switch shape {
	case tableTest:
		body = []string{
			"describe('" + title + " " + tag + "', () => {",
			"  let current" + name + ": Cart;",
			"  beforeEach(() => { current" + name + " = make" + name + "(); });",
			"  it.each([[7], [8]])('" + title + " %s " + tag + "', (row) => {",
			"    // " + title,
			"    expect(current" + name + ".size + row).toBe(limit" + name + ");",
			"  });",
			"});",
		}
	case tableSuiteTest:
		body = []string{
			"describe.each([[7]])('" + title + " %s', (row) => {",
			"  it('" + title + " " + tag + "', () => {",
			"    expect(make" + name + "().size + row).toBe(limit" + name + ");",
			"  });",
			"});",
		}
	default:
		body = []string{
			"describe('" + title + " " + tag + "', () => {",
			"  it('" + title + " " + tag + "', () => {",
			"    // " + title,
			"    expect(make" + name + "().size).toBe(limit" + name + ");",
			"  });",
			"});",
		}
	}
	return strings.Join(append(header, body...), "\n") + "\n"
}

// reportedName spells the name a runner gives the test of propertySource.
func reportedName(shape testShape, words []string, refs []criterion.Ref, vitest bool) string {
	title := strings.Join(words, " ")
	tag := fmt.Sprintf(tagged("[cites %s]"), refs[0])
	var parts []string
	switch shape {
	case tableTest:
		parts = []string{title + " " + tag, title + " " + rowValue + " " + tag}
	case tableSuiteTest:
		parts = []string{title + " " + rowValue, title + " " + tag}
	default:
		parts = []string{title + " " + tag, title + " " + tag}
	}
	if vitest {
		return strings.Join(parts, " > ")
	}
	return strings.Join(parts, " ")
}

func TestMaskedInputNeverLeaks(t *testing.T) {
	t.Run("no run of its titles, marker word or criterion id survives for any title, any shape and any runner [verifies SP-1204~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			words := drawWords(t)
			refs := drawRefs(t)
			shape := testShape(rapid.IntRange(0, int(shapeCount)-1).Draw(t, "shape"))
			vitest := rapid.Bool().Draw(t, "vitest")
			project := fstest.MapFS{
				sampleTestFile: {Data: []byte(propertySource(shape, words, refs))},
				"src/shop.ts":  {Data: []byte(shopLibrary)},
			}
			test := criterion.BoundTest{Classname: sampleTestFile, Name: reportedName(shape, words, refs, vitest), File: sampleTestFile}

			masked, err := javascript.New().Mask(project, test)
			if err != nil {
				t.Fatalf("mask %s: %v\n%s", test.Label(), err, project[sampleTestFile].Data)
			}
			name := testname.Compact(test.Name)
			if len(masking.NameRuns(strings.Join(append(words, words...), " "), name)) == 0 {
				t.Fatalf("the name %q is not found in its own words %v, so the check below proves nothing", name, words)
			}
			if leaks := masking.Check(masked.Unit, name, refs); len(leaks) > 0 {
				t.Fatalf("the unit leaks %v:\n%s", leaks, masked.Unit)
			}
			if leaks := masking.Check(masked.Signatures, name, refs); len(leaks) > 0 {
				t.Fatalf("the signatures leak %v:\n%s", leaks, masked.Signatures)
			}
			if !strings.Contains(masked.Unit, "'case'") {
				t.Fatalf("the titles are not neutral:\n%s", masked.Unit)
			}
		})
	})

	t.Run("the check reads the whole name of a Vitest test, which a bracket of a citation must not cut short [verifies SP-1204~1]", func(t *testing.T) {
		reported := tagged("alpha bravo charlie [cites PAR-1~1] > delta echo foxtrot [cites PAR-1~1]")
		name := testname.Compact(reported)
		if name != "alpha bravo charlie delta echo foxtrot" {
			t.Fatalf("name %q", name)
		}
		if runs := masking.NameRuns("delta echo foxtrot", name); len(runs) != 1 {
			t.Fatalf("runs %v", runs)
		}
		if runs := masking.NameRuns("delta echo foxtrot", reported); len(runs) != 0 {
			t.Fatalf("the unprocessed name was read whole: %v", runs)
		}
	})
}
