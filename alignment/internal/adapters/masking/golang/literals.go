package golang

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
)

const (
	neutralSubtestName = "case"
	rawStringQuote     = "`"
)

// literalNeutralizer rewrites the string literals of a unit: citation markers
// are removed, the matched subtest name and any literal that repeats a run of
// the test name become the neutral subtest name.
type literalNeutralizer struct {
	subtest  *ast.BasicLit
	testName string
	tags     map[*ast.BasicLit]bool
}

// neutralizeLiterals rewrites the string literals of the declarations.
func neutralizeLiterals(decls []*declaration, subtest *ast.BasicLit, testName string) {
	n := &literalNeutralizer{subtest: subtest, testName: testName, tags: map[*ast.BasicLit]bool{}}
	for _, decl := range decls {
		n.collectTags(decl.node)
		ast.Inspect(decl.node, func(node ast.Node) bool {
			if literal, isLiteral := node.(*ast.BasicLit); isLiteral && literal.Kind == token.STRING && !n.tags[literal] {
				n.rewrite(literal)
			}
			return true
		})
	}
}

func (n *literalNeutralizer) collectTags(node ast.Node) {
	ast.Inspect(node, func(child ast.Node) bool {
		if field, isField := child.(*ast.Field); isField && field.Tag != nil {
			n.tags[field.Tag] = true
		}
		return true
	})
}

func (n *literalNeutralizer) rewrite(literal *ast.BasicLit) {
	if literal == n.subtest {
		literal.Value = strconv.Quote(neutralSubtestName)
		return
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return
	}
	cleaned := removeCitations(value)
	if len(masking.NameRuns(cleaned, n.testName)) > 0 {
		literal.Value = strconv.Quote(neutralSubtestName)
		return
	}
	if cleaned != value {
		literal.Value = requote(literal.Value, cleaned)
	}
}

// requote writes the value as a literal of the same style as the original.
func requote(original, value string) string {
	if strings.HasPrefix(original, rawStringQuote) && !strings.Contains(value, rawStringQuote) {
		return rawStringQuote + value + rawStringQuote
	}
	return strconv.Quote(value)
}
