package javascript

import (
	"regexp"
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/syntax"
	"github.com/RachidChabane/spec-proof/alignment/internal/platform/testname"
)

// callKind tells what a call statement of a test file is in the Jest and Vitest
// vocabulary.
type callKind int

const (
	// callOther is any statement that is none of the kinds below.
	callOther callKind = iota
	// callSuite groups tests: `describe`, `describe.each(table)`.
	callSuite
	// callTest is a test: `test`, `it`, `it.each(table)`.
	callTest
	// callHook runs around the tests of its suite: `beforeEach`, `afterAll`.
	callHook
	// callSetup configures the framework or the mocks of its suite: `jest.mock`,
	// `vi.useFakeTimers`.
	callSetup
)

// Names the runners give their call vocabulary.
var (
	suiteNames = []string{"describe", "suite", "context"}
	testNames  = []string{"it", "test", "specify"}
	hookNames  = []string{"beforeEach", "afterEach", "beforeAll", "afterAll", "before", "after"}
	setupNames = []string{"jest", "vi"}
	// tableModifiers turn a suite or a test into one per row of a table.
	tableModifiers = []string{"each", "for"}
)

// Placeholders of a title: the printf-style ones and `$field` of a table row are
// those of a table-driven call, `${}` of a template literal those of any call.
var (
	tablePlaceholders    = regexp.MustCompile(`%[a-zA-Z#%]|\$\{[^}]*\}|\$[A-Za-z_#][\w.#]*`)
	templatePlaceholders = regexp.MustCompile(`\$\{[^}]*\}`)
)

// bracketBlanker keeps a bracket of a title from cutting the name the leak check
// reads.
var bracketBlanker = strings.NewReplacer("[", " ", "]", " ")

const (
	neutralTitle = "'case'"
	wholeTitle   = `.+`
)

// call is a call statement of a test file read as part of the runner's
// vocabulary.
type call struct {
	kind callKind
	// statement is the expression statement, node the call that runs the
	// callback.
	statement, node *gotreesitter.Node
	// title is the first argument of the call and callback its function, nil
	// when the call has none.
	title, callback *gotreesitter.Node
	tableDriven     bool
}

// classify reads a statement as a call of the runner's vocabulary.
func (x *fileIndex) classify(statement *gotreesitter.Node) call {
	if x.Type(statement) != typeExpressionStatment || statement.NamedChildCount() == 0 {
		return call{kind: callOther, statement: statement}
	}
	invocation := statement.NamedChild(0)
	if x.Type(invocation) != typeCallExpression {
		return call{kind: callOther, statement: statement}
	}
	found := call{statement: statement, node: invocation}
	target := x.Field(invocation, fieldFunction)
	for x.Type(target) == typeCallExpression {
		found.tableDriven = true
		target = x.Field(target, fieldFunction)
	}
	root, modifiers := x.chainOf(target)
	found.tableDriven = found.tableDriven || slices.ContainsFunc(modifiers, func(m string) bool { return slices.Contains(tableModifiers, m) })
	found.kind = kindOf(root, len(modifiers) > 0)
	if found.kind == callSuite || found.kind == callTest {
		found.title, found.callback = x.invocationParts(invocation)
		if found.title == nil {
			found.kind = callOther
		}
	}
	return found
}

func kindOf(root string, modified bool) callKind {
	switch {
	case slices.Contains(suiteNames, root):
		return callSuite
	case slices.Contains(testNames, root):
		return callTest
	case slices.Contains(hookNames, root) && !modified:
		return callHook
	case slices.Contains(setupNames, root):
		return callSetup
	}
	return callOther
}

// chainOf reads `root.modifier.modifier` and returns the root identifier and the
// modifiers in order.
func (x *fileIndex) chainOf(target *gotreesitter.Node) (root string, modifiers []string) {
	for x.Type(target) == typeMemberExpression {
		modifiers = append([]string{x.Text(x.Field(target, fieldProperty))}, modifiers...)
		target = x.Field(target, fieldObject)
	}
	if x.Type(target) != typeIdentifier {
		return "", nil
	}
	return x.Text(target), modifiers
}

// invocationParts finds the title and the callback among the arguments.
func (x *fileIndex) invocationParts(invocation *gotreesitter.Node) (title, callback *gotreesitter.Node) {
	arguments := x.Field(invocation, fieldArguments)
	if x.Type(arguments) != typeArguments {
		return nil, nil
	}
	named := x.Named(arguments)
	if len(named) == 0 {
		return nil, nil
	}
	title = named[0]
	for _, candidate := range named[1:] {
		switch x.Type(candidate) {
		case typeArrowFunction, typeFunctionExpression, typeFunctionOld:
			callback = candidate
		}
	}
	return title, callback
}

// titleOf reads the title of a call: its text with the quotes removed, and whether
// the title is built at run time and so can stand for any text.
func (x *fileIndex) titleOf(title *gotreesitter.Node) (text string, dynamic bool) {
	switch x.Type(title) {
	case typeString:
		return x.stringContent(title), false
	case typeTemplateString:
		content := x.Text(title)
		return strings.Trim(content, "`"), false
	}
	return "", true
}

func (x *fileIndex) stringContent(node *gotreesitter.Node) string {
	var content strings.Builder
	for _, child := range x.Named(node) {
		content.WriteString(x.Text(child))
	}
	return content.String()
}

// titlePattern is the regular expression source that matches the names a title can
// expand to, once its citations are removed.
func (x *fileIndex) titlePattern(c call) string {
	text, dynamic := x.titleOf(c.title)
	if dynamic {
		return wholeTitle
	}
	placeholders := templatePlaceholders
	if c.tableDriven {
		placeholders = tablePlaceholders
	}
	return testname.PatternOf(testname.Compact(text), placeholders)
}

// titleWords is the title as its author wrote it, for the check that the masked
// input does not spell it: the citations and the placeholders are removed.
func (x *fileIndex) titleWords(c call) string {
	text, dynamic := x.titleOf(c.title)
	if dynamic {
		return ""
	}
	return strings.TrimSpace(bracketBlanker.Replace(tablePlaceholders.ReplaceAllString(testname.Compact(text), " ")))
}

// suiteBlock is the statements of the callback of a suite, nil when the callback
// has no block body.
func (x *fileIndex) suiteBlock(c call) *gotreesitter.Node {
	if c.callback == nil {
		return nil
	}
	if body := x.Field(c.callback, fieldBody); x.Type(body) == typeStatementBlock {
		return body
	}
	return nil
}

// lineSpan is the 1-based first and last line of a call statement.
func lineSpan(c call) (first, last int) {
	return syntax.Line(c.statement), syntax.EndLine(c.statement)
}
