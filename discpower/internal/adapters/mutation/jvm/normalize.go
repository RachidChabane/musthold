package jvm

import (
	"errors"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// ErrSyntax is returned for a source that does not parse.
var ErrSyntax = errors.New("java or kotlin syntax error")

// canonical serializes a tree as nested lists of node types with the text of the
// leaves, leaving out comments. Layout does not appear in it, and block
// structure does.
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
	for i := 0; i < node.ChildCount(); i++ {
		if child := node.Child(i); !p.dialect.isComment(p.typeOf(child)) {
			children = append(children, child)
		}
	}
	return children
}
