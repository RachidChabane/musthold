package node

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

// ErrUnmappedTest is returned for a bound test that names no test file.
var ErrUnmappedTest = errors.New("bound test cannot be mapped to a file")

const (
	alternation  = "|"
	anchoredOne  = "^%s$"
	anchoredMany = "^(?:%s)$"
	// plainTitleSeparator is the space a title filter may join titles with.
	plainTitleSeparator = " "
	resultsExtension    = ".xml"
)

// titleSelection is what both runners select by: the files of the bound tests
// and one anchored regex over their full titles.
type titleSelection struct {
	files      []string
	expression string
	tests      []ecosystem.TestID
}

// collectTitles reads the files and the full titles of the bound tests. quote
// turns the name the report gives a test into the regex that matches its full
// title.
func collectTitles(bound []ecosystem.BoundTest, quote func(name string) string) (titleSelection, error) {
	var selection titleSelection
	var expressions []string
	for _, test := range bound {
		if test.File == "" {
			return titleSelection{}, fmt.Errorf("%w: %s", ErrUnmappedTest, test.ID)
		}
		if !slices.Contains(selection.files, test.File) {
			selection.files = append(selection.files, test.File)
		}
		if expression := quote(test.Name); !slices.Contains(expressions, expression) {
			expressions = append(expressions, expression)
		}
		selection.tests = append(selection.tests, test.ID)
	}
	selection.expression = anchor(expressions)
	return selection, nil
}

func anchor(expressions []string) string {
	if len(expressions) == 1 {
		return fmt.Sprintf(anchoredOne, expressions[0])
	}
	return fmt.Sprintf(anchoredMany, strings.Join(expressions, alternation))
}

// quoteTitle escapes a full title for a regex. A non-empty separator is the
// text the report joins titles with; it matches the separator or a space.
func quoteTitle(name, separator string) string {
	if separator == "" {
		return regexp.QuoteMeta(name)
	}
	parts := strings.Split(name, separator)
	for index, part := range parts {
		parts[index] = regexp.QuoteMeta(part)
	}
	joiner := "(?:" + regexp.QuoteMeta(separator) + alternation + regexp.QuoteMeta(plainTitleSeparator) + ")"
	return strings.Join(parts, joiner)
}

// selectByTitle builds the selection of the bound tests; arguments assembles
// the runner's arguments from the files and the expression.
func selectByTitle(bound []ecosystem.BoundTest, quote func(name string) string, arguments func(titleSelection) []string) (ecosystem.Selection, error) {
	if len(bound) == 0 {
		return ecosystem.Selection{}, nil
	}
	titles, err := collectTitles(bound, quote)
	if err != nil {
		return ecosystem.Selection{}, err
	}
	return ecosystem.Selection{Args: arguments(titles), Tests: titles.tests}, nil
}

// dropUnselected removes the tests a title filter reported as skipped without
// selecting them. A run without arguments is a full run and keeps every outcome.
func dropUnselected(result ecosystem.RunResult, selection ecosystem.Selection) ecosystem.RunResult {
	if len(selection.Args) == 0 {
		return result
	}
	for id, outcome := range result.Outcomes {
		if outcome == ecosystem.TestSkipped && !slices.Contains(selection.Tests, id) {
			delete(result.Outcomes, id)
		}
	}
	return result
}
