package golang

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	subtestSeparator = "/"
	runMethodName    = "Run"
	// minimumRunArguments is the number of arguments of a subtest call: its name
	// and its body.
	minimumRunArguments = 2
)

var (
	// citationPattern matches a citation marker inside a string.
	citationPattern = regexp.MustCompile(`(?i)\s*\[verifies[^\]]*\]`)
	// reportedCitationPattern matches a citation marker in a name where Go
	// replaced the spaces by underscores.
	reportedCitationPattern = regexp.MustCompile(`(?i)_*\[verifies[^\]]*\]`)
	// uniquifierPattern matches the `#01` suffix Go appends to a repeated name.
	uniquifierPattern = regexp.MustCompile(`#[0-9]+$`)
)

// testPath is the path of a reported test name: the top-level function and the
// first subtest element.
type testPath struct {
	function string
	subtest  string
}

func parseTestName(name string) testPath {
	elements := strings.Split(name, subtestSeparator)
	path := testPath{function: uniquifierPattern.ReplaceAllString(elements[0], "")}
	if len(elements) > 1 {
		path.subtest = elements[1]
	}
	return path
}

// subtestKey is the comparable form of the subtest element, empty when the
// bound test is a top-level test.
func (p testPath) subtestKey() string {
	if p.subtest == "" {
		return ""
	}
	return canonicalSubtestName(uniquifierPattern.ReplaceAllString(p.subtest, ""))
}

// removeCitations drops the citation markers of a string.
func removeCitations(text string) string { return citationPattern.ReplaceAllString(text, "") }

// canonicalSubtestName is the name Go reports for a subtest title (spaces
// replaced by underscores), without citation markers.
func canonicalSubtestName(title string) string {
	reported := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return '_'
		}
		return r
	}, title)
	return reportedCitationPattern.ReplaceAllString(reported, "")
}

// subtestLiteral returns the name literal of a `X.Run(<string literal>, body)`
// call.
func subtestLiteral(node ast.Node) (*ast.BasicLit, bool) {
	call, isCall := node.(*ast.CallExpr)
	if !isCall || len(call.Args) < minimumRunArguments {
		return nil, false
	}
	method, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || method.Sel.Name != runMethodName {
		return nil, false
	}
	literal, isLiteral := call.Args[0].(*ast.BasicLit)
	return literal, isLiteral && literal.Kind == token.STRING
}

// selectSubtest finds the single `Run` call of the function whose literal name
// is the bound subtest, removes the sibling `Run` statements named by another
// literal, and returns the matched literal. It returns nil, leaving the function
// whole, when no literal or several literals match.
func selectSubtest(fn *ast.FuncDecl, key string) *ast.BasicLit {
	if key == "" || fn.Body == nil {
		return nil
	}
	var matches []*ast.BasicLit
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if literal, found := subtestLiteral(node); found && literalKey(literal) == key {
			matches = append(matches, literal)
		}
		return true
	})
	if len(matches) != 1 {
		return nil
	}
	dropSiblingRuns(fn.Body, matches[0])
	return matches[0]
}

func literalKey(literal *ast.BasicLit) string {
	title, err := strconv.Unquote(literal.Value)
	if err != nil {
		return ""
	}
	return canonicalSubtestName(title)
}

// dropSiblingRuns removes, from the statement list that holds the kept `Run`
// statement, every other `Run` statement named by a literal.
func dropSiblingRuns(body *ast.BlockStmt, kept *ast.BasicLit) {
	ast.Inspect(body, func(node ast.Node) bool {
		list := statementList(node)
		if list != nil && holdsRun(*list, kept) {
			*list = withoutOtherRuns(*list, kept)
		}
		return true
	})
}

func statementList(node ast.Node) *[]ast.Stmt {
	switch n := node.(type) {
	case *ast.BlockStmt:
		return &n.List
	case *ast.CaseClause:
		return &n.Body
	case *ast.CommClause:
		return &n.Body
	}
	return nil
}

func runStatementLiteral(stmt ast.Stmt) (*ast.BasicLit, bool) {
	expression, isExpression := stmt.(*ast.ExprStmt)
	if !isExpression {
		return nil, false
	}
	return subtestLiteral(expression.X)
}

func holdsRun(list []ast.Stmt, kept *ast.BasicLit) bool {
	for _, stmt := range list {
		if literal, isRun := runStatementLiteral(stmt); isRun && literal == kept {
			return true
		}
	}
	return false
}

func withoutOtherRuns(list []ast.Stmt, kept *ast.BasicLit) []ast.Stmt {
	var remaining []ast.Stmt
	for _, stmt := range list {
		if literal, isRun := runStatementLiteral(stmt); isRun && literal != kept {
			continue
		}
		remaining = append(remaining, stmt)
	}
	return remaining
}
