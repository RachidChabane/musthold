// Package syntax wraps a tree-sitter parse of a source file with the accessors the
// language maskers read it through.
package syntax

import (
	"fmt"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/textedit"
)

// Tree is a parsed source file. The grammar recovers from syntax errors, so a
// broken file still yields a tree.
type Tree struct {
	Lang   *gotreesitter.Language
	Source []byte
	tree   *gotreesitter.Tree
}

// Parse parses the source with the grammar.
func Parse(lang *gotreesitter.Language, source []byte) (*Tree, error) {
	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}
	return &Tree{Lang: lang, Source: source, tree: tree}, nil
}

// Release frees the tree; no node of it may be used afterwards.
func (t *Tree) Release() { t.tree.Release() }

// Root is the root node of the file.
func (t *Tree) Root() *gotreesitter.Node { return t.tree.RootNode() }

// Type is the grammar type of the node, empty for nil.
func (t *Tree) Type(n *gotreesitter.Node) string {
	if n == nil {
		return ""
	}
	return n.Type(t.Lang)
}

// Text is the source text of the node, empty for nil.
func (t *Tree) Text(n *gotreesitter.Node) string {
	if n == nil {
		return ""
	}
	return n.Text(t.Source)
}

// Field is the child the node holds under the field name, nil when it has none.
func (t *Tree) Field(n *gotreesitter.Node, field string) *gotreesitter.Node {
	if n == nil {
		return nil
	}
	return n.ChildByFieldName(field, t.Lang)
}

// Children lists every child of the node, named or not.
func (t *Tree) Children(n *gotreesitter.Node) []*gotreesitter.Node {
	if n == nil {
		return nil
	}
	children := make([]*gotreesitter.Node, 0, n.ChildCount())
	for index := 0; index < n.ChildCount(); index++ {
		children = append(children, n.Child(index))
	}
	return children
}

// Named lists the named children of the node.
func (t *Tree) Named(n *gotreesitter.Node) []*gotreesitter.Node {
	if n == nil {
		return nil
	}
	children := make([]*gotreesitter.Node, 0, n.NamedChildCount())
	for index := 0; index < n.NamedChildCount(); index++ {
		children = append(children, n.NamedChild(index))
	}
	return children
}

// FieldChildren lists the children the node holds under the field name.
func (t *Tree) FieldChildren(n *gotreesitter.Node, field string) []*gotreesitter.Node {
	var found []*gotreesitter.Node
	for index := 0; index < n.ChildCount(); index++ {
		if n.FieldNameForChild(index, t.Lang) == field {
			found = append(found, n.Child(index))
		}
	}
	return found
}

// FirstOfType is the first named child of the node with the grammar type, nil
// when there is none.
func (t *Tree) FirstOfType(n *gotreesitter.Node, kind string) *gotreesitter.Node {
	for _, child := range t.Named(n) {
		if t.Type(child) == kind {
			return child
		}
	}
	return nil
}

// Span is the byte range of the node.
func Span(n *gotreesitter.Node) textedit.Span {
	return textedit.Span{Start: int(n.StartByte()), End: int(n.EndByte())}
}

// Line is the 1-based line the node starts on.
func Line(n *gotreesitter.Node) int { return int(n.StartPoint().Row) + 1 }

// EndLine is the 1-based line the node ends on.
func EndLine(n *gotreesitter.Node) int { return int(n.EndPoint().Row) + 1 }

// Visitor is called for a node with its parent and the field it holds in it; it
// returns whether the walk goes on into the node's children.
type Visitor func(node, parent *gotreesitter.Node, field string) bool

// Walk visits the node and its descendants in source order.
func (t *Tree) Walk(n *gotreesitter.Node, visit Visitor) { t.walk(n, nil, "", visit) }

func (t *Tree) walk(n, parent *gotreesitter.Node, field string, visit Visitor) {
	if !visit(n, parent, field) {
		return
	}
	for index := 0; index < n.ChildCount(); index++ {
		t.walk(n.Child(index), n, n.FieldNameForChild(index, t.Lang), visit)
	}
}
