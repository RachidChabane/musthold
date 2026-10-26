package python_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/alignment/internal/adapters/masking/python"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
)

// nameVocabulary holds words that no fixture of the property spells.
var nameVocabulary = []string{
	"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliet",
}

// parameterID is the id pytest gives the parameter set of a parametrized test:
// the values joined by dashes.
const parameterID = "10-200-180.0"

const (
	minNameWords   = 1
	maxNameWords   = 8
	maxCriteria    = 4
	propertyModule = "tests.test_property"
	propertyFile   = "tests/test_property.py"
	cartLibrary    = "class Cart:\n    def __init__(self, size: int) -> None:\n        self.size = size\n"
)

type testShape int

const (
	moduleFunction testShape = iota
	classMethod
	parametrizedFunction
	shapeCount
)

func drawName(t *rapid.T) string {
	words := rapid.SliceOfN(rapid.SampledFrom(nameVocabulary), minNameWords, maxNameWords).Draw(t, "words")
	return "test_" + strings.Join(words, "_")
}

func drawRefs(t *rapid.T) []criterion.Ref {
	ids := rapid.SliceOfN(rapid.StringMatching(`[A-Z]{2,4}-[1-9][0-9]{0,3}`), 1, maxCriteria).Draw(t, "ids")
	refs := make([]criterion.Ref, len(ids))
	for index, id := range ids {
		refs[index] = criterion.Ref{ID: id, Revision: 1}
	}
	return refs
}

// propertySource builds a test file around a test name: the name, its words and
// the ids appear in a marker, a docstring, a comment, a helper and a fixture.
func propertySource(shape testShape, name string, refs []criterion.Ref) string {
	words := strings.TrimPrefix(name, "test_")
	marker := fmt.Sprintf("@pytest.mark.verifies(%q)", refs[0].String())
	header := []string{
		"import pytest",
		"from shop import Cart",
		"",
		"def make_" + words + "():",
		`    """Makes the ` + words + `."""`,
		"    return Cart(2)  # " + words,
		"",
		"@pytest.fixture",
		"def " + words + "_fixture():",
		"    return make_" + words + "()",
		"",
	}
	var body []string
	switch shape {
	case classMethod:
		body = []string{
			"class Test" + strings.ToUpper(words[:1]) + ":",
			"    " + marker,
			"    def " + name + "(self, " + words + "_fixture):",
			`        """` + name + " " + refs[0].ID + `"""`,
			"        # " + name,
			"        assert " + words + "_fixture.size",
		}
	case parametrizedFunction:
		body = []string{
			marker,
			`@pytest.mark.parametrize("size,low,high", [(10, 200, 180.0), (1, 2, 3.5)])`,
			"def " + name + "(" + words + "_fixture, size, low, high):",
			"    # " + words,
			"    assert " + words + "_fixture.size + size + low + high",
		}
	default:
		body = []string{
			marker,
			"def " + name + "(" + words + "_fixture):",
			`    """` + words + " " + refs[0].ID + `"""`,
			"    # " + words,
			"    assert " + words + "_fixture.size",
		}
	}
	return strings.Join(append(header, body...), "\n") + "\n"
}

func boundTestOf(shape testShape, name string) criterion.BoundTest {
	switch shape {
	case classMethod:
		words := strings.TrimPrefix(name, "test_")
		return criterion.BoundTest{Classname: propertyModule + ".Test" + strings.ToUpper(words[:1]), Name: name, File: propertyFile}
	case parametrizedFunction:
		return criterion.BoundTest{Classname: propertyModule, Name: name + "[" + parameterID + "]", File: propertyFile}
	default:
		return criterion.BoundTest{Classname: propertyModule, Name: name, File: propertyFile}
	}
}

func TestParameterIDsAreNotLeaks(t *testing.T) {
	const source = `import pytest
from shop import Cart

@pytest.mark.verifies("PAR-2~1")
@pytest.mark.parametrize("size,low,high", [(10, 200, 180.0), (1, 2, 3.5)])
def test_total(size, low, high):
    assert Cart(size).size + low + high
`
	project := fstest.MapFS{
		propertyFile:      {Data: []byte(source)},
		"src/shop.py":     {Data: []byte(cartLibrary)},
		"src/__init__.py": {Data: []byte("")},
	}

	t.Run("the ids of a parametrized run are not a run of the test name [verifies SP-905~1]", func(t *testing.T) {
		test := criterion.BoundTest{Classname: propertyModule, Name: "test_total[" + parameterID + "]", File: propertyFile}
		masked, err := python.New().Mask(project, test)
		if err != nil {
			t.Fatalf("mask %s: %v", test.Label(), err)
		}
		if !strings.Contains(masked.Unit, "10, 200, 180.0") {
			t.Fatalf("the parameters were dropped:\n%s", masked.Unit)
		}
		if leaks := masking.Check(masked.Unit, test.Name, nil); len(leaks) > 0 {
			t.Fatalf("the unit leaks %v:\n%s", leaks, masked.Unit)
		}
	})

	t.Run("a run of words of the name still leaks when the test is parametrized [verifies SP-905~1]", func(t *testing.T) {
		leaking := fstest.MapFS{
			propertyFile: {Data: []byte("import pytest\n\n@pytest.mark.parametrize(\"size\", [10])\ndef test_grace_period_is_free(size):\n    label = \"grace period is free\"\n    assert size, label\n")},
		}
		test := criterion.BoundTest{Classname: propertyModule, Name: "test_grace_period_is_free[10]", File: propertyFile}
		if _, err := python.New().Mask(leaking, test); !errors.Is(err, masking.ErrLeak) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestMaskedInputNeverLeaks(t *testing.T) {
	t.Run("no run of its name, marker word or criterion id survives for any test name [verifies SP-325~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			name := drawName(t)
			refs := drawRefs(t)
			shape := testShape(rapid.IntRange(0, int(shapeCount)-1).Draw(t, "shape"))
			project := fstest.MapFS{
				propertyFile:      {Data: []byte(propertySource(shape, name, refs))},
				"src/shop.py":     {Data: []byte(cartLibrary)},
				"src/__init__.py": {Data: []byte("")},
			}
			test := boundTestOf(shape, name)

			masked, err := python.New().Mask(project, test)
			if err != nil {
				t.Fatalf("mask %s: %v\n%s", test.Label(), err, project[propertyFile].Data)
			}
			if leaks := masking.Check(masked.Unit, test.Name, refs); len(leaks) > 0 {
				t.Fatalf("the unit leaks %v:\n%s", leaks, masked.Unit)
			}
			if leaks := masking.Check(masked.Signatures, test.Name, refs); len(leaks) > 0 {
				t.Fatalf("the signatures leak %v:\n%s", leaks, masked.Signatures)
			}
			if !strings.Contains(masked.Unit, "def test_case(") {
				t.Fatalf("the test is not renamed:\n%s", masked.Unit)
			}
		})
	})
}
