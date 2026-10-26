package java_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"unicode"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/alignment/internal/adapters/masking/java"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
	"github.com/RachidChabane/musthold/alignment/internal/platform/testname"
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
	shopLibrary  = "package shop;\n\npublic class Cart {\n    public int size() { return 2; }\n}\n"
)

type testShape int

const (
	displayNamed testShape = iota
	parametrized
	nestedClass
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
// the display names with a citation, in a comment, in the names of a constant, of
// a helper method and of the test method.
func propertySource(shape testShape, words []string, refs []criterion.Ref) string {
	title := strings.Join(words, " ")
	tag := fmt.Sprintf(tagged("[cites %s]"), refs[0])
	name := capitalized(words)
	method := "test" + name
	header := []string{
		"package shop;",
		"",
		"class SampleTest {",
		"    static final int LIMIT" + name + " = 3;",
		"",
		"    static Cart make" + name + "() {",
		"        // " + title + " " + refs[0].ID,
		"        return new Cart();",
		"    }",
		"",
	}
	var body []string
	switch shape {
	case parametrized:
		body = []string{
			"    @ParameterizedTest(name = \"" + title + " {0} " + tag + "\")",
			"    @ValueSource(ints = {7})",
			"    void " + method + "(int row) {",
			"        // " + title,
			"        assertEquals(LIMIT" + name + ", make" + name + "().size() + row);",
			"    }",
		}
	case nestedClass:
		body = []string{
			"    @Nested",
			"    @DisplayName(\"" + title + " " + tag + "\")",
			"    class Inner" + name + " {",
			"        @Test",
			"        void " + method + "() {",
			"            assertEquals(LIMIT" + name + ", make" + name + "().size());",
			"        }",
			"    }",
		}
	default:
		body = []string{
			"    @Test",
			"    @DisplayName(\"" + title + " " + tag + "\")",
			"    void " + method + "() {",
			"        // " + title,
			"        assertEquals(LIMIT" + name + ", make" + name + "().size());",
			"    }",
		}
	}
	return strings.Join(append(append(header, body...), "}"), "\n") + "\n"
}

// boundTestOf spells the test a runner reports for the test of propertySource.
func boundTestOf(shape testShape, words []string, refs []criterion.Ref) criterion.BoundTest {
	title := strings.Join(words, " ")
	tag := fmt.Sprintf(tagged("[cites %s]"), refs[0])
	method := "test" + capitalized(words)
	const class = "shop.SampleTest"
	switch shape {
	case parametrized:
		return criterion.BoundTest{Classname: class, Name: title + " " + rowValue + " " + tag, File: sampleTestFile}
	case nestedClass:
		return criterion.BoundTest{Classname: class + "$Inner" + capitalized(words), Name: method + "()", File: sampleTestFile}
	}
	return criterion.BoundTest{Classname: class, Name: title + " " + tag, File: sampleTestFile}
}

func TestMaskedInputNeverLeaks(t *testing.T) {
	t.Run("no run of its names, marker word or criterion id survives for any name and any shape of test [verifies SP-1213~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			words := drawWords(t)
			refs := drawRefs(t)
			shape := testShape(rapid.IntRange(0, int(shapeCount)-1).Draw(t, "shape"))
			project := fstest.MapFS{
				sampleTestFile:                 {Data: []byte(propertySource(shape, words, refs))},
				"src/main/java/shop/Cart.java": {Data: []byte(shopLibrary)},
			}
			test := boundTestOf(shape, words, refs)

			masked, err := java.New().Mask(project, test)
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
			if !strings.Contains(masked.Unit, "testCase()") && !strings.Contains(masked.Unit, "testCase(int row)") {
				t.Fatalf("the test is not renamed:\n%s", masked.Unit)
			}
		})
	})
}
