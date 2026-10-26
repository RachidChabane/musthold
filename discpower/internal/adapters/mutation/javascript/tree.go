package javascript

import (
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/platform/syntaxtree"
)

// Node types of the tree-sitter JavaScript and TypeScript grammars.
const (
	typeFunctionDeclaration  = "function_declaration"
	typeMethod               = "method_definition"
	typeClassDeclaration     = "class_declaration"
	typeLexicalDeclaration   = "lexical_declaration"
	typeVariableDeclaration  = "variable_declaration"
	typeDeclarator           = "variable_declarator"
	typeExportStatement      = "export_statement"
	typeImportStatement      = "import_statement"
	typeImportSpecifier      = "import_specifier"
	typeIdentifier           = "identifier"
	typePropertyIdentifier   = "property_identifier"
	typeTypeIdentifier       = "type_identifier"
	typeSimpleIdentifier     = "simple_identifier"
	typeShorthand            = "shorthand_property_identifier"
	typeShorthandPattern     = "shorthand_property_identifier_pattern"
	typeNumber               = "number"
	typeString               = "string"
	typeStringFragment       = "string_fragment"
	typeComment              = "comment"
	typeBinary               = "binary_expression"
	typeAugmented            = "augmented_assignment_expression"
	typeIf                   = "if_statement"
	typeElse                 = "else_clause"
	typeWhile                = "while_statement"
	typeDoWhile              = "do_statement"
	typeFor                  = "for_statement"
	typeTernary              = "ternary_expression"
	typeReturn               = "return_statement"
	typeThrow                = "throw_statement"
	typeNew                  = "new_expression"
	typeCall                 = "call_expression"
	typeMember               = "member_expression"
	typeArguments            = "arguments"
	typePair                 = "pair"
	typeObject               = "object"
	typeTrue                 = "true"
	typeFalse                = "false"
	typeLiteralType          = "literal_type"
	typeParenthesized        = "parenthesized_expression"
	typeStatementBlock       = "statement_block"
	typeExpressionStatement  = "expression_statement"
	typeEmptyStatement       = "empty_statement"
	typeWith                 = "with_statement"
	typeForIn                = "for_in_statement"
	typeCatch                = "catch_clause"
	typeThis                 = "this"
	typeSuper                = "super"
	typeArrowFunction        = "arrow_function"
	typeFunctionExpression   = "function_expression"
	typeGeneratorFunction    = "generator_function"
	typeGeneratorDeclaration = "generator_function_declaration"
	typeClassExpression      = "class"
	typeAbstractClass        = "abstract_class_declaration"
	typeSemicolon            = ";"
)

// Field names of the grammars.
const (
	fieldName        = "name"
	fieldAlias       = "alias"
	fieldCondition   = "condition"
	fieldBody        = "body"
	fieldOperator    = "operator"
	fieldLeft        = "left"
	fieldRight       = "right"
	fieldFunction    = "function"
	fieldConstructor = "constructor"
	fieldArguments   = "arguments"
	fieldObject      = "object"
	fieldProperty    = "property"
	fieldValue       = "value"
	fieldKind        = "kind"
	fieldParameters  = "parameters"
	fieldParameter   = "parameter"
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
	path   string
	lang   *gotreesitter.Language
	tree   *gotreesitter.Tree
	source []byte
}

func parse(filePath string, source []byte) (*parsed, error) {
	lang, err := grammarFor(filePath)
	if err != nil {
		return nil, err
	}
	tree, err := syntaxtree.Parse(lang, source)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return &parsed{path: filePath, lang: lang, tree: tree, source: source}, nil
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

func (p *parsed) spanText(span ecosystem.Span) string { return string(p.source[span.Start:span.End]) }

// firstNamed returns the first named child of n that is not a comment.
func (p *parsed) firstNamed(n *gotreesitter.Node) *gotreesitter.Node {
	for i := 0; i < n.NamedChildCount(); i++ {
		if child := n.NamedChild(i); p.typeOf(child) != typeComment {
			return child
		}
	}
	return nil
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

// calleeName is the last name of a call or construction: `f` for `f(...)` and
// `new f(...)`, `m` for `x.m(...)`; empty for any other callee.
func (p *parsed) calleeName(call *gotreesitter.Node) string {
	callee := p.child(call, fieldFunction)
	if callee == nil {
		callee = p.child(call, fieldConstructor)
	}
	if callee == nil {
		return ""
	}
	switch p.typeOf(callee) {
	case typeIdentifier:
		return p.text(callee)
	case typeMember:
		if property := p.child(callee, fieldProperty); property != nil {
			return p.text(property)
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
