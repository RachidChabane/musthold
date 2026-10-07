package javascript

import (
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/testlabel"
	"github.com/RachidChabane/spec-proof/completeness/internal/platform/treeunits"
)

// callKind tells what a call statement of a test file is in the vocabulary of the
// runners.
type callKind int

const (
	callOther callKind = iota
	callSuite
	callTest
	callHook
	callSetup
)

// call is a call statement read in the vocabulary of the runners.
type call struct {
	kind callKind
	node *gotreesitter.Node
	// title is the first argument of the call and callback its function.
	title, callback *gotreesitter.Node
	tableDriven     bool
}

// suite is a `describe` the test is nested in, with its title as written and the
// pattern of the labels it adds.
type suite struct {
	title, pattern string
}

// tests lists the tests among the statements of a block, with the suites above
// them. The setup statements of the blocks around a test run with it, so what they
// mention counts as mentioned by the test.
func (w *walker) tests(statements []*gotreesitter.Node, path []suite, setup []*gotreesitter.Node) {
	here := slices.Clone(setup)
	for _, statement := range statements {
		if w.runsAroundTests(statement) {
			here = append(here, statement)
		}
	}
	for _, statement := range statements {
		read := w.classify(statement)
		switch read.kind {
		case callSuite:
			w.suite(read, path, here)
		case callTest:
			w.test(read, path, here)
		}
	}
}

// runsAroundTests reports whether a statement of a block is set up for its tests: a
// hook, a framework call such as a mock, or a declaration.
func (w *walker) runsAroundTests(statement *gotreesitter.Node) bool {
	switch w.classify(statement).kind {
	case callHook, callSetup:
		return true
	case callOther:
		switch w.tree.Type(statement) {
		case typeLexical, typeVariable, typeFunctionDecl, typeClassDeclaration:
			return true
		}
	}
	return false
}

func (w *walker) suite(read call, path []suite, setup []*gotreesitter.Node) {
	body := w.blockOf(read)
	if body == nil {
		return
	}
	title := w.titleText(read)
	if title == "" {
		title = dynamicTitle
	}
	inner := append(slices.Clone(path), suite{title: title, pattern: w.pattern(read)})
	w.tests(w.statementsOf(body), inner, setup)
}

func (w *walker) test(read call, path []suite, setup []*gotreesitter.Node) {
	titles := make([]string, 0, len(path)+1)
	patterns := make([]string, 0, len(path)+1)
	for _, enclosing := range path {
		titles = append(titles, enclosing.title)
		patterns = append(patterns, enclosing.pattern)
	}
	title, pattern := w.titleText(read), w.pattern(read)
	if title == "" {
		title = dynamicTitle
	}
	titles, patterns = append(titles, title), append(patterns, pattern)
	patterns = slices.DeleteFunc(patterns, func(p string) bool { return p == "" })
	w.units.Add(treeunits.Spec{
		Node: read.node, Qualname: strings.Join(titles, titleSeparator), Name: title, Kind: code.KindTest,
		Also: setup, Labels: []string{strings.Join(patterns, patternSeparator)},
	})
}

// classify reads a statement as a call of the vocabulary of the runners.
func (w *walker) classify(statement *gotreesitter.Node) call {
	if w.tree.Type(statement) != typeExpressionStatment || statement.NamedChildCount() == 0 {
		return call{kind: callOther, node: statement}
	}
	invocation := statement.NamedChild(0)
	if w.tree.Type(invocation) != typeCallExpression {
		return call{kind: callOther, node: statement}
	}
	found := call{node: statement}
	target := w.tree.Field(invocation, fieldFunction)
	for w.tree.Type(target) == typeCallExpression {
		found.tableDriven = true
		target = w.tree.Field(target, fieldFunction)
	}
	root, modifiers := w.chainOf(target)
	found.tableDriven = found.tableDriven || slices.ContainsFunc(modifiers, func(m string) bool { return slices.Contains(tableModifiers, m) })
	found.kind = kindOf(root, len(modifiers) > 0)
	if found.kind == callSuite || found.kind == callTest {
		found.title, found.callback = w.invocationParts(invocation)
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
func (w *walker) chainOf(target *gotreesitter.Node) (root string, modifiers []string) {
	for w.tree.Type(target) == typeMemberExpression {
		modifiers = append([]string{w.tree.Text(w.tree.Field(target, fieldProperty))}, modifiers...)
		target = w.tree.Field(target, fieldObject)
	}
	if w.tree.Type(target) != typeIdentifier {
		return "", nil
	}
	return w.tree.Text(target), modifiers
}

func (w *walker) invocationParts(invocation *gotreesitter.Node) (title, callback *gotreesitter.Node) {
	named := w.tree.Named(w.tree.Field(invocation, fieldArguments))
	if len(named) == 0 {
		return nil, nil
	}
	title = named[0]
	for _, candidate := range named[1:] {
		if w.isFunction(candidate) {
			callback = candidate
		}
	}
	return title, callback
}

// blockOf is the statement block of the callback of a suite.
func (w *walker) blockOf(read call) *gotreesitter.Node {
	if read.callback == nil {
		return nil
	}
	if body := w.tree.Field(read.callback, fieldBody); w.tree.Type(body) == typeStatementBlock {
		return body
	}
	return nil
}

// titleText is the title of a call as written, empty when it is built at run time.
func (w *walker) titleText(read call) string {
	switch w.tree.Type(read.title) {
	case typeString:
		var content strings.Builder
		for _, part := range w.tree.Named(read.title) {
			content.WriteString(w.tree.Text(part))
		}
		return content.String()
	case typeTemplateString:
		return strings.Trim(w.tree.Text(read.title), "`")
	}
	return ""
}

// pattern is the regular expression source that matches the labels a title can
// expand to; a title built at run time matches anything.
func (w *walker) pattern(read call) string {
	text := w.titleText(read)
	if text == "" {
		return wholeTitle
	}
	placeholders := templatePlaceholders
	if read.tableDriven {
		placeholders = tablePlaceholders
	}
	return testlabel.PatternOf(text, placeholders)
}
