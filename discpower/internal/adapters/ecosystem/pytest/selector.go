package pytest

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// ErrUnmappedTest is returned for a bound test no definition of its file explains.
var ErrUnmappedTest = errors.New("bound test cannot be mapped to a definition")

const (
	nodeSeparator     = "::"
	qualnameSeparator = "."
	parameterOpen     = "["
	parameterClose    = "]"
	pathSeparator     = "/"
)

type selector struct{}

// Select maps each bound test to a pytest node id. The report's classname and
// name name the test (module, classes, function, parameter id); the locator
// confirms the definition exists in the bound file. When the classname does not
// belong to the file, the bound source line is mapped to the enclosing or
// following definition instead.
func (selector) Select(bound []ecosystem.BoundTest, definitions map[string][]ecosystem.Definition) (ecosystem.Selection, error) {
	var selection ecosystem.Selection
	for _, test := range bound {
		qualname, params, ok := resolve(test, definitions[test.File])
		if !ok {
			return ecosystem.Selection{}, fmt.Errorf("%w: %s (%s:%d)", ErrUnmappedTest, test.ID, test.File, test.Line)
		}
		node := test.File + nodeSeparator + strings.ReplaceAll(qualname, qualnameSeparator, nodeSeparator) + params
		if !slices.Contains(selection.Args, node) {
			selection.Args = append(selection.Args, node)
		}
		selection.Tests = append(selection.Tests, test.ID)
	}
	return selection, nil
}

func resolve(test ecosystem.BoundTest, definitions []ecosystem.Definition) (qualname, params string, ok bool) {
	function, params := splitParameters(test.Name)
	if classes, belongs := classesOf(test); belongs {
		candidate := strings.Join(append(classes, function), qualnameSeparator)
		for _, definition := range definitions {
			if definition.Qualname == candidate {
				return candidate, params, true
			}
		}
	}
	definition, found := ecosystem.EnclosingDefinition(definitions, test.Line)
	if !found {
		return "", "", false
	}
	return definition.Qualname, params, true
}

// splitParameters separates `test_x[1-2]` into `test_x` and `[1-2]`.
func splitParameters(name string) (function, params string) {
	open := strings.Index(name, parameterOpen)
	if open < 0 || !strings.HasSuffix(name, parameterClose) {
		return name, ""
	}
	return name[:open], name[open:]
}

// classesOf returns the classes between the module and the function when the
// classname starts with the dotted module path of the bound file.
func classesOf(test ecosystem.BoundTest) ([]string, bool) {
	module := strings.ReplaceAll(strings.TrimSuffix(test.File, pythonSuffix), pathSeparator, qualnameSeparator)
	switch {
	case test.Classname == module:
		return nil, true
	case strings.HasPrefix(test.Classname, module+qualnameSeparator):
		return strings.Split(strings.TrimPrefix(test.Classname, module+qualnameSeparator), qualnameSeparator), true
	default:
		return nil, false
	}
}
