package jvm

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

// ErrUnmappedTest is returned for a bound test no definition of its file explains.
var ErrUnmappedTest = errors.New("bound test cannot be mapped to a definition")

const (
	qualnameSeparator = "."
	nestedSeparator   = "$"
	packageSeparator  = "."
	backtick          = "`"
)

// sourcePath matches a source file below a source set, capturing the path of the
// file under the language directory, which is the package path then the file.
var sourcePath = regexp.MustCompile(`(?:^|/)src/[^/]+/(?:java|kotlin)/(.+)\.(?:java|kt)$`)

// testTarget is what one bound test asks the build tool to run: a method of a
// class, or every method of the class when the bound test is not one method.
type testTarget struct {
	class string
	// method is empty for a whole class.
	method string
}

type selector struct {
	tool buildTool
}

// Select maps each bound test to a class or a method through its source
// location. A source line inside a method (its annotations are part of it, so a
// `@DisplayName` or `@ParameterizedTest` line is) selects that method; a line
// outside every method, such as the `@DisplayName` of a class, selects the whole
// class of the next method, because Shallnot reports one location for every test
// of the class.
func (s selector) Select(bound []ecosystem.BoundTest, definitions map[string][]ecosystem.Definition) (ecosystem.Selection, error) {
	var selection ecosystem.Selection
	var targets []testTarget
	for _, test := range bound {
		target, err := targetOf(test, definitions[test.File])
		if err != nil {
			return ecosystem.Selection{}, err
		}
		if !slices.Contains(targets, target) {
			targets = append(targets, target)
		}
		selection.Tests = append(selection.Tests, test.ID)
	}
	selection.Args = s.tool.filters(targets)
	return selection, nil
}

func targetOf(test ecosystem.BoundTest, definitions []ecosystem.Definition) (testTarget, error) {
	unmapped := fmt.Errorf("%w: %s (%s:%d)", ErrUnmappedTest, test.ID, test.File, test.Line)
	definition, found := ecosystem.EnclosingDefinition(definitions, test.Line)
	if !found {
		return testTarget{}, unmapped
	}
	class, method, ok := splitQualname(definition.Qualname)
	if !ok {
		return testTarget{}, unmapped
	}
	name, ok := binaryName(test.File, class)
	if !ok {
		return testTarget{}, unmapped
	}
	if definition.StartLine <= test.Line && test.Line <= definition.EndLine {
		return testTarget{class: name, method: method}, nil
	}
	return testTarget{class: name}, nil
}

// splitQualname separates `Outer.Inner.method` into the classes and the method.
func splitQualname(qualname string) (classes []string, method string, ok bool) {
	parts := strings.Split(qualname, qualnameSeparator)
	if len(parts) < 2 {
		return nil, "", false
	}
	return parts[:len(parts)-1], strings.Trim(parts[len(parts)-1], backtick), true
}

// binaryName is the binary name of a class declared in a file: the package its
// path below the source root spells, then the outer class and the nested ones
// joined by `$`.
func binaryName(file string, classes []string) (string, bool) {
	match := sourcePath.FindStringSubmatch(file)
	if match == nil {
		return "", false
	}
	packagePath := path.Dir(match[1])
	name := strings.Join(classes, nestedSeparator)
	if packagePath == "." {
		return name, true
	}
	return strings.ReplaceAll(packagePath, "/", packageSeparator) + packageSeparator + name, true
}

// classTargets is the methods to run in one class; none means the whole class.
type classTargets struct {
	name    string
	methods []string
}

// classesOf groups targets by class in order of first appearance. A class asked
// for as a whole absorbs the methods asked for in it.
func classesOf(targets []testTarget) []classTargets {
	var grouped []classTargets
	for _, target := range targets {
		index := slices.IndexFunc(grouped, func(c classTargets) bool { return c.name == target.class })
		if index < 0 {
			grouped = append(grouped, classTargets{name: target.class})
			index = len(grouped) - 1
		}
		if target.method != "" && !slices.Contains(grouped[index].methods, target.method) {
			grouped[index].methods = append(grouped[index].methods, target.method)
		}
	}
	for index, class := range grouped {
		if slices.ContainsFunc(targets, func(t testTarget) bool { return t.class == class.name && t.method == "" }) {
			grouped[index].methods = nil
		}
	}
	return grouped
}
