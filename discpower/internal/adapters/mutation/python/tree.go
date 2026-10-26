package python

import (
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/platform/syntaxtree"
)

// Node types of the tree-sitter Python grammar.
const (
	typeModule              = "module"
	typeClass               = "class_definition"
	typeFunction            = "function_definition"
	typeAssignment          = "assignment"
	typeExpressionStatement = "expression_statement"
	typeIdentifier          = "identifier"
	typeAttribute           = "attribute"
	typeCall                = "call"
	typeArgumentList        = "argument_list"
	typeKeywordArgument     = "keyword_argument"
	typeString              = "string"
	typeStringContent       = "string_content"
	typeComment             = "comment"
	typeBlock               = "block"
	typeInteger             = "integer"
	typeFloat               = "float"
	typeTrue                = "true"
	typeFalse               = "false"
	typeComparison          = "comparison_operator"
	typeBinary              = "binary_operator"
	typeBoolean             = "boolean_operator"
	typeIf                  = "if_statement"
	typeElif                = "elif_clause"
	typeElse                = "else_clause"
	typeWhile               = "while_statement"
	typeTernary             = "conditional_expression"
	typeReturn              = "return_statement"
	typeRaise               = "raise_statement"
	typeExcept              = "except_clause"
	typeAsPattern           = "as_pattern"
	typeLambda              = "lambda"
	typeParameters          = "parameters"
	typeGlobal              = "global_statement"
	typeNonlocal            = "nonlocal_statement"
	typeFor                 = "for_statement"
	typeForInClause         = "for_in_clause"
	typeIfToken             = "if"
)

// Field names of the grammar.
const (
	fieldName      = "name"
	fieldLeft      = "left"
	fieldRight     = "right"
	fieldOperator  = "operator"
	fieldOperators = "operators"
	fieldFunction  = "function"
	fieldArguments = "arguments"
	fieldValue     = "value"
	fieldObject    = "object"
	fieldAttribute = "attribute"
	fieldCondition = "condition"
	fieldBody      = "body"
	fieldParams    = "parameters"
)

// ancestry is the chain of nodes from the root down to the visited node, the
// visited node excluded.
type ancestry []*gotreesitter.Node

func (a ancestry) parent() *gotreesitter.Node { return a.up(1) }

// up returns the n-th ancestor, 1 being the parent.
func (a ancestry) up(n int) *gotreesitter.Node {
	if n > len(a) {
		return nil
	}
	return a[len(a)-n]
}

// parsed is a parsed source file.
type parsed struct {
	lang   *gotreesitter.Language
	tree   *gotreesitter.Tree
	source []byte
}

func parse(source []byte) (*parsed, error) {
	lang := grammars.PythonLanguage()
	tree, err := syntaxtree.Parse(lang, source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return &parsed{lang: lang, tree: tree, source: source}, nil
}

func (p *parsed) release() { p.tree.Release() }

func (p *parsed) root() *gotreesitter.Node { return p.tree.RootNode() }

func (p *parsed) typeOf(n *gotreesitter.Node) string { return n.Type(p.lang) }

func (p *parsed) text(n *gotreesitter.Node) string { return n.Text(p.source) }

func (p *parsed) fieldOf(parent *gotreesitter.Node, index int) string {
	return parent.FieldNameForChild(index, p.lang)
}

func (p *parsed) child(n *gotreesitter.Node, field string) *gotreesitter.Node {
	return n.ChildByFieldName(field, p.lang)
}

func (p *parsed) span(n *gotreesitter.Node) ecosystem.Span {
	return ecosystem.Span{Start: int(n.StartByte()), End: int(n.EndByte())}
}

// walk visits every node below n, depth first, with its field name in its
// parent and its ancestors. When visit returns false the node's children are
// skipped.
func (p *parsed) walk(n *gotreesitter.Node, visit func(node *gotreesitter.Node, field string, up ancestry) bool) {
	p.walkFrom(n, "", nil, visit)
}

func (p *parsed) walkFrom(n *gotreesitter.Node, field string, up ancestry, visit func(*gotreesitter.Node, string, ancestry) bool) {
	if !visit(n, field, up) {
		return
	}
	down := append(up[:len(up):len(up)], n)
	for i := 0; i < n.ChildCount(); i++ {
		p.walkFrom(n.Child(i), p.fieldOf(n, i), down, visit)
	}
}

// identifiers collects the text of the identifiers below n.
func (p *parsed) identifiers(n *gotreesitter.Node) map[string]bool {
	found := map[string]bool{}
	p.walk(n, func(node *gotreesitter.Node, _ string, _ ancestry) bool {
		if p.typeOf(node) == typeIdentifier {
			found[p.text(node)] = true
		}
		return true
	})
	return found
}

// calleeName is the last name of a call's function: `f` for `f(...)`, `m` for
// `x.m(...)`; empty for any other callee.
func (p *parsed) calleeName(call *gotreesitter.Node) string {
	callee := p.child(call, fieldFunction)
	if callee == nil {
		return ""
	}
	switch p.typeOf(callee) {
	case typeIdentifier:
		return p.text(callee)
	case typeAttribute:
		if attribute := p.child(callee, fieldAttribute); attribute != nil {
			return p.text(attribute)
		}
	}
	return ""
}

// replaceIn returns unit with the absolute span replaced; unitStart is the
// absolute offset of the unit.
func replaceIn(unit string, unitStart int, span ecosystem.Span, replacement string) string {
	return unit[:span.Start-unitStart] + replacement + unit[span.End-unitStart:]
}

func isUpper(name string) bool {
	return name != "" && strings.ToUpper(name[:1]) == name[:1] && strings.ToLower(name[:1]) != name[:1]
}
