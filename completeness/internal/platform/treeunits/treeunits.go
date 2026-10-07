// Package treeunits builds the units of a source file from its tree-sitter parse:
// the text of a node, its normal form, which ignores formatting and comments, the
// names it mentions and its lines. A per-language indexer decides which nodes are
// units; this package gives each the same shape.
package treeunits

import (
	"fmt"
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
)

// Spellings of the normal form.
const (
	structureOpen  = "("
	structureClose = ")"
	tokenSeparator = " "
	repeatFormat   = "%s#%d"
	backtick       = "`"
)

// Tree is a parsed source file. The grammar recovers from syntax errors, so a broken
// file still yields a tree.
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

// FirstOfType is the first named child of the node with one of the grammar types,
// nil when there is none.
func (t *Tree) FirstOfType(n *gotreesitter.Node, kinds ...string) *gotreesitter.Node {
	for _, child := range t.Named(n) {
		if slices.Contains(kinds, t.Type(child)) {
			return child
		}
	}
	return nil
}

// Rules tell a language's comments and identifiers apart from the rest of its tree.
type Rules struct {
	// Comments are the node types left out of the normal form of a unit.
	Comments []string
	// Identifiers are the node types whose text is a name a unit mentions.
	Identifiers []string
}

// Collector lists the units of one file. A unit that shares its qualified name with
// an earlier one gets a repeat counter, so that every unit of the file is named
// once.
type Collector struct {
	tree  *Tree
	file  string
	rules Rules
	names map[string]int
	units []code.Unit
}

// NewCollector starts the units of a file.
func NewCollector(tree *Tree, file string, rules Rules) *Collector {
	return &Collector{tree: tree, file: file, rules: rules, names: map[string]int{}}
}

// Spec describes a unit: the node that carries its text, its names and its kind.
type Spec struct {
	Node     *gotreesitter.Node
	Qualname string
	Name     string
	Owner    string
	Kind     code.Kind
	// Also lists nodes outside the unit whose names the unit mentions too, such as
	// the hooks that run before a test.
	Also []*gotreesitter.Node
	// Labels are the patterns of a test unit.
	Labels []string
}

// Add appends the unit the spec describes.
func (c *Collector) Add(spec Spec) {
	mentioned := map[string]bool{}
	c.mentions(spec.Node, mentioned)
	for _, extra := range spec.Also {
		c.mentions(extra, mentioned)
	}
	names := make([]string, 0, len(mentioned))
	for name := range mentioned {
		names = append(names, name)
	}
	slices.Sort(names)
	var normal strings.Builder
	c.normalize(spec.Node, &normal)
	c.units = append(c.units, code.Unit{
		File: c.file, Qualname: c.unique(spec.Qualname), Name: spec.Name, Owner: spec.Owner, Kind: spec.Kind,
		Normalized: normal.String(), Text: c.tree.Text(spec.Node),
		StartLine: int(spec.Node.StartPoint().Row) + 1, EndLine: int(spec.Node.EndPoint().Row) + 1,
		Mentions: names, Labels: spec.Labels,
	})
}

// Units lists the units added, in the order they were added.
func (c *Collector) Units() []code.Unit { return c.units }

// unique gives a unit a qualified name that is unique in its file.
func (c *Collector) unique(qualname string) string {
	c.names[qualname]++
	if count := c.names[qualname]; count > 1 {
		return fmt.Sprintf(repeatFormat, qualname, count)
	}
	return qualname
}

// normalize writes the syntax tree of a node as nested parentheses over its leaves,
// leaving out the comments.
func (c *Collector) normalize(node *gotreesitter.Node, out *strings.Builder) {
	if slices.Contains(c.rules.Comments, c.tree.Type(node)) {
		return
	}
	if node.ChildCount() == 0 {
		out.WriteString(c.tree.Text(node))
		out.WriteString(tokenSeparator)
		return
	}
	out.WriteString(structureOpen + c.tree.Type(node) + tokenSeparator)
	for index := 0; index < node.ChildCount(); index++ {
		c.normalize(node.Child(index), out)
	}
	out.WriteString(structureClose + tokenSeparator)
}

// mentions records every identifier of a node, which covers calls, member names and
// parameter names alike.
func (c *Collector) mentions(node *gotreesitter.Node, found map[string]bool) {
	kind := c.tree.Type(node)
	if slices.Contains(c.rules.Comments, kind) {
		return
	}
	if slices.Contains(c.rules.Identifiers, kind) {
		found[strings.Trim(c.tree.Text(node), backtick)] = true
		return
	}
	for index := 0; index < node.ChildCount(); index++ {
		c.mentions(node.Child(index), found)
	}
}
