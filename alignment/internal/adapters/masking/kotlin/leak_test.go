package kotlin_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"unicode"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/masking/kotlin"
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
	rowValue     = "7"
	shopLibrary  = "package shop\n\nclass Cart {\n    fun size(): Int = 2\n}\n"
)

type testShape int

const (
	displayNamed testShape = iota
	parametrized
	nestedClass
	sentenceNamed
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

func capitalized(words []string) string {
	var out strings.Builder
	for _, word := range words {
		out.WriteString(string(unicode.ToUpper(rune(word[0]))) + word[1:])
	}
	return out.String()
}

// propertySource builds a test class around the words of a name: they appear in
// the display names with a citation, in a comment, in the names of a constant, of a
// helper function and of the test function.
func propertySource(shape testShape, words []string, refs []criterion.Ref) string {
	title := strings.Join(words, " ")
	tag := fmt.Sprintf(tagged("[cites %s]"), refs[0])
	name := capitalized(words)
	function := "test" + name
	header := []string{
		"package shop",
		"",
		"class SampleTest {",
		"    val limit" + name + " = 3",
		"",
		"    fun make" + name + "() = Cart().also {",
		"        // " + title + " " + refs[0].ID,
		"    }",
		"",
	}
	var body []string
	switch shape {
	case parametrized:
		body = []string{
			"    @ParameterizedTest(name = \"" + title + " {0} " + tag + "\")",
			"    @ValueSource(ints = [7])",
			"    fun " + function + "(row: Int) {",
			"        // " + title,
			"        assertEquals(limit" + name + ", make" + name + "().size() + row)",
			"    }",
		}
	case nestedClass:
		body = []string{
			"    @Nested",
			"    @DisplayName(\"" + title + " " + tag + "\")",
			"    inner class Inner" + name + " {",
			"        @Test",
			"        fun " + function + "() {",
			"            assertEquals(limit" + name + ", make" + name + "().size())",
			"        }",
			"    }",
		}
	case sentenceNamed:
		body = []string{
			"    @Test",
			"    fun `" + title + "`() {",
			"        // " + title,
			"        assertEquals(limit" + name + ", make" + name + "().size())",
			"    }",
		}
	default:
		body = []string{
			"    @Test",
			"    @DisplayName(\"" + title + " " + tag + "\")",
			"    fun " + function + "() {",
			"        // " + title,
			"        assertEquals(limit" + name + ", make" + name + "().size())",
			"    }",
		}
	}
	return strings.Join(append(append(header, body...), "}"), "\n") + "\n"
}

// boundTestOf spells the test a runner reports for the test of propertySource.
func boundTestOf(shape testShape, words []string, refs []criterion.Ref) criterion.BoundTest {
	title := strings.Join(words, " ")
	tag := fmt.Sprintf(tagged("[cites %s]"), refs[0])
	const class = "shop.SampleTest"
	switch shape {
	case parametrized:
		return criterion.BoundTest{Classname: class, Name: title + " " + rowValue + " " + tag, File: sampleTestFile}
	case nestedClass:
		return criterion.BoundTest{Classname: class + "$Inner" + capitalized(words), Name: "test" + capitalized(words) + "()", File: sampleTestFile}
	case sentenceNamed:
		return criterion.BoundTest{Classname: class, Name: title + "()", File: sampleTestFile}
	}
	return criterion.BoundTest{Classname: class, Name: title + " " + tag, File: sampleTestFile}
}

func TestMaskedInputNeverLeaks(t *testing.T) {
	t.Run("no run of its names, marker word or criterion id survives for any name and any shape of test [verifies SP-1223~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			words := drawWords(t)
			refs := drawRefs(t)
			shape := testShape(rapid.IntRange(0, int(shapeCount)-1).Draw(t, "shape"))
			project := fstest.MapFS{
				sampleTestFile:                 {Data: []byte(propertySource(shape, words, refs))},
				"src/main/kotlin/shop/Cart.kt": {Data: []byte(shopLibrary)},
			}
			test := boundTestOf(shape, words, refs)

			masked, err := kotlin.New().Mask(project, test)
			if err != nil {
				t.Fatalf("mask %s: %v\n%s", test.Label(), err, project[sampleTestFile].Data)
			}
			doubled := strings.Join(append(append([]string{}, words...), words...), " ")
			names := []string{testname.Compact(test.Name), "test" + capitalized(words)}
			for _, name := range names {
				if len(masking.NameRuns(doubled, name)) == 0 {
					t.Fatalf("the name %q is not found in its own words %v, so the check below proves nothing", name, words)
				}
				if leaks := masking.Check(masked.Unit, name, refs); len(leaks) > 0 {
					t.Fatalf("the unit leaks %v:\n%s", leaks, masked.Unit)
				}
				if leaks := masking.Check(masked.Signatures, name, refs); len(leaks) > 0 {
					t.Fatalf("the signatures leak %v:\n%s", leaks, masked.Signatures)
				}
			}
			if !strings.Contains(masked.Unit, "fun testCase(") {
				t.Fatalf("the test is not renamed:\n%s", masked.Unit)
			}
		})
	})
}
