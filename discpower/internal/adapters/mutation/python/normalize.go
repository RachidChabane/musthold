package python

import (
	"errors"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// ErrSyntax is returned for a source that does not parse.
var ErrSyntax = errors.New("python syntax error")

// canonical serializes a tree as nested lists of node types with the text of the
// leaves, leaving out comments and docstrings. Layout and indentation do not
// appear in it, and block structure does.
func (p *parsed) canonical() string {
	var out strings.Builder
	p.writeCanonical(&out, p.root())
	return out.String()
}

func (p *parsed) writeCanonical(out *strings.Builder, node *gotreesitter.Node) {
	children := p.canonicalChildren(node)
	out.WriteByte('(')
	out.WriteString(p.typeOf(node))
	if len(children) == 0 {
		out.WriteByte(' ')
		out.WriteString(p.text(node))
	}
	for _, child := range children {
		p.writeCanonical(out, child)
	}
	out.WriteByte(')')
}

func (p *parsed) canonicalChildren(node *gotreesitter.Node) []*gotreesitter.Node {
	var children []*gotreesitter.Node
	docstringPossible := p.typeOf(node) == typeBlock || p.typeOf(node) == typeModule
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if p.typeOf(child) == typeComment {
			continue
		}
		if docstringPossible && child.IsNamed() {
			docstringPossible = false
			if p.isDocstring(child) {
				continue
			}
		}
		children = append(children, child)
	}
	return children
}

func (p *parsed) isDocstring(statement *gotreesitter.Node) bool {
	switch p.typeOf(statement) {
	case typeString:
		return true
	case typeExpressionStatement:
		return statement.NamedChildCount() == 1 && p.typeOf(statement.NamedChild(0)) == typeString
	}
	return false
}
