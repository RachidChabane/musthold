package golang

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
)

// ErrUnmappedTest is returned for a bound test whose top-level test function is
// not declared in its file.
var ErrUnmappedTest = errors.New("bound test cannot be mapped to a definition")

const (
	subtestSeparator = "/"
	alternation      = "|"
	anchoredOne      = "^%s$"
	anchoredMany     = "^(%s)$"
	selectionFlag    = runFlag
	qualnameDot      = "."
)

type selector struct{}

// Select builds one `-run` expression and the package list for the bound tests.
//
// Every bound name is a subtest path (`TestX/sub_name`); each path element is
// anchored and the elements of one level are alternated. The expression stops
// at the shallowest depth among the bound tests, so a bound parent test always
// runs its subtests in full. Tests of the selected levels that are not bound may
// run too; their outcomes are never read.
func (selector) Select(bound []ecosystem.BoundTest, definitions map[string][]ecosystem.Definition) (ecosystem.Selection, error) {
	if len(bound) == 0 {
		return ecosystem.Selection{}, nil
	}
	paths := make([][]string, 0, len(bound))
	var packages []string
	selection := ecosystem.Selection{}
	for _, test := range bound {
		elements := strings.Split(test.Name, subtestSeparator)
		if !declares(definitions[test.File], elements[0]) {
			return ecosystem.Selection{}, fmt.Errorf("%w: %s (%s)", ErrUnmappedTest, test.ID, test.File)
		}
		paths = append(paths, elements)
		if pkg := packagePath(path.Dir(test.File)); !slices.Contains(packages, pkg) {
			packages = append(packages, pkg)
		}
		selection.Tests = append(selection.Tests, test.ID)
	}
	slices.Sort(packages)
	selection.Args = append([]string{selectionFlag, expression(paths)}, packages...)
	return selection, nil
}

// declares reports whether a top-level function with the name exists.
func declares(definitions []ecosystem.Definition, name string) bool {
	return slices.ContainsFunc(definitions, func(d ecosystem.Definition) bool {
		return d.Kind == ecosystem.KindFunction && strings.HasSuffix(d.Qualname, qualnameDot+name)
	})
}

func expression(paths [][]string) string {
	depth := len(paths[0])
	for _, elements := range paths {
		depth = min(depth, len(elements))
	}
	levels := make([]string, depth)
	for level := range depth {
		var names []string
		for _, elements := range paths {
			if quoted := regexp.QuoteMeta(elements[level]); !slices.Contains(names, quoted) {
				names = append(names, quoted)
			}
		}
		levels[level] = anchor(names)
	}
	return strings.Join(levels, subtestSeparator)
}

func anchor(names []string) string {
	if len(names) == 1 {
		return fmt.Sprintf(anchoredOne, names[0])
	}
	return fmt.Sprintf(anchoredMany, strings.Join(names, alternation))
}
