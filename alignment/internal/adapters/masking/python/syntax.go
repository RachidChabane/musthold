package python

import (
	"fmt"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Node types of the tree-sitter Python grammar.
const (
	typeAliasedImport       = "aliased_import"
	typeAssignment          = "assignment"
	typeAttribute           = "attribute"
	typeBlock               = "block"
	typeCall                = "call"
	typeClass               = "class_definition"
	typeColon               = ":"
	typeComment             = "comment"
	typeDecorated           = "decorated_definition"
	typeDecorator           = "decorator"
	typeExpressionStatement = "expression_statement"
	typeFunction            = "function_definition"
	typeIdentifier          = "identifier"
	typeImport              = "import_statement"
	typeImportFrom          = "import_from_statement"
	typeKeywordArgument     = "keyword_argument"
	typeString              = "string"
)

// Field names of the grammar.
const (
	fieldAlias      = "alias"
	fieldAttribute  = "attribute"
	fieldBody       = "body"
	fieldDefinition = "definition"
	fieldFunction   = "function"
	fieldLeft       = "left"
	fieldModule     = "module_name"
	fieldName       = "name"
	fieldObject     = "object"
	fieldType       = "type"
)

// parsed is a parsed Python source file.
type parsed struct {
	lang   *gotreesitter.Language
	tree   *gotreesitter.Tree
	source []byte
}

// parseSource parses Python source. The grammar recovers from syntax errors, so
// a broken file still yields a tree.
func parseSource(source []byte) (*parsed, error) {
	lang := grammars.PythonLanguage()
	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse python source: %w", err)
	}
	return &parsed{lang: lang, tree: tree, source: source}, nil
}

func (p *parsed) release() { p.tree.Release() }

func (p *parsed) root() *gotreesitter.Node { return p.tree.RootNode() }

func (p *parsed) typeOf(n *gotreesitter.Node) string {
	if n == nil {
		return ""
	}
	return n.Type(p.lang)
}

func (p *parsed) text(n *gotreesitter.Node) string {
	if n == nil {
		return ""
	}
	return n.Text(p.source)
}

func (p *parsed) child(n *gotreesitter.Node, field string) *gotreesitter.Node {
	return n.ChildByFieldName(field, p.lang)
}

// childrenOf lists every child of the node, named or not.
func (p *parsed) childrenOf(n *gotreesitter.Node) []*gotreesitter.Node {
	children := make([]*gotreesitter.Node, 0, n.ChildCount())
	for i := 0; i < n.ChildCount(); i++ {
		children = append(children, n.Child(i))
	}
	return children
}

// statementsOf lists the named children of the node that are not comments.
func (p *parsed) statementsOf(n *gotreesitter.Node) []*gotreesitter.Node {
	var statements []*gotreesitter.Node
	for i := 0; i < n.NamedChildCount(); i++ {
		if child := n.NamedChild(i); p.typeOf(child) != typeComment {
			statements = append(statements, child)
		}
	}
	return statements
}

// fieldChildren lists the children that the node holds under the field.
func (p *parsed) fieldChildren(n *gotreesitter.Node, field string) []*gotreesitter.Node {
	var found []*gotreesitter.Node
	for i := 0; i < n.ChildCount(); i++ {
		if n.FieldNameForChild(i, p.lang) == field {
			found = append(found, n.Child(i))
		}
	}
	return found
}

// bare returns the expression of an expression statement, and any other node
// unchanged. Some grammar builds fold the wrapper away.
func (p *parsed) bare(n *gotreesitter.Node) *gotreesitter.Node {
	if p.typeOf(n) == typeExpressionStatement && n.NamedChildCount() == 1 {
		return n.NamedChild(0)
	}
	return n
}

// definitionOf returns the function or class a top-level statement defines,
// looking through its decorators, or nil.
func (p *parsed) definitionOf(statement *gotreesitter.Node) *gotreesitter.Node {
	if p.typeOf(statement) == typeDecorated {
		statement = p.child(statement, fieldDefinition)
	}
	switch p.typeOf(statement) {
	case typeFunction, typeClass:
		return statement
	}
	return nil
}

func spanOf(n *gotreesitter.Node) span {
	return span{start: int(n.StartByte()), end: int(n.EndByte())}
}
