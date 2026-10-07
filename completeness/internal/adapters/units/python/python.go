// Package python indexes the units of Python source files with the pure-Go
// tree-sitter runtime: functions, methods and module-level assignments. The
// normal form of a unit is the stream of its syntax tree's leaves, without
// comments and docstrings, so that a change of formatting, of a comment or of a
// docstring is no change.
package python

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
)

// Naming rules of Python source files.
const (
	sourceExtension = ".py"
	testPrefix      = "test_"
	testSuffix      = "_test.py"
	conftestName    = "conftest.py"
	qualifierFormat = "%s.%s"
	repeatFormat    = "%s#%d"
)

// testDirectories are the directory names whose files hold tests.
var testDirectories = []string{"tests", "test"}

// Node types and fields of the tree-sitter Python grammar.
const (
	typeAssignment      = "assignment"
	typeBlock           = "block"
	typeClass           = "class_definition"
	typeComment         = "comment"
	typeDecorated       = "decorated_definition"
	typeExpression      = "expression_statement"
	typeFunction        = "function_definition"
	typeIdentifier      = "identifier"
	typeString          = "string"
	fieldBody           = "body"
	fieldDefinition     = "definition"
	fieldLeft           = "left"
	fieldName           = "name"
	structureOpen       = "("
	structureClose      = ")"
	tokenSeparator      = " "
	singleNamedChildren = 1
)

var _ ports.Indexer = Indexer{}

// Indexer implements ports.Indexer for Python.
type Indexer struct{}

// New returns the Python indexer.
func New() Indexer { return Indexer{} }

// Handles reports whether the file is a Python source file.
func (Indexer) Handles(file string) bool { return path.Ext(file) == sourceExtension }

// IsTest reports whether the file holds Python tests: a `test_*.py`, `*_test.py`
// or `conftest.py` file, or a file below a `tests` or `test` directory.
func (Indexer) IsTest(file string) bool {
	base := path.Base(file)
	if strings.HasPrefix(base, testPrefix) || strings.HasSuffix(base, testSuffix) || base == conftestName {
		return true
	}
	return slices.ContainsFunc(strings.Split(path.Dir(file), "/"), func(segment string) bool {
		return slices.Contains(testDirectories, segment)
	})
}

// Index parses the source and lists its units in source order. The grammar
// recovers from syntax errors, so a broken file still yields the units it could
// read.
func (i Indexer) Index(file string, source []byte) (code.File, error) {
	lang := grammars.PythonLanguage()
	tree, err := gotreesitter.NewParser(lang).Parse(source)
	if err != nil {
		return code.File{}, fmt.Errorf("parse %s: %w", file, err)
	}
	defer tree.Release()
	walk := &walker{file: file, lang: lang, source: source, names: map[string]int{}}
	walk.body(tree.RootNode(), "", "")
	return code.File{Path: file, Test: i.IsTest(file), Units: walk.units}, nil
}

type walker struct {
	file   string
	lang   *gotreesitter.Language
	source []byte
	units  []code.Unit
	names  map[string]int
}

func (w *walker) typeOf(node *gotreesitter.Node) string { return node.Type(w.lang) }

func (w *walker) text(node *gotreesitter.Node) string { return node.Text(w.source) }

// qualname gives a unit a name that is unique in its file: a redefinition gets a
// repeat counter.
func (w *walker) qualname(name string) string {
	w.names[name]++
	if count := w.names[name]; count > 1 {
		return fmt.Sprintf(repeatFormat, name, count)
	}
	return name
}

// body lists the units among the statements of a module or class body.
func (w *walker) body(container *gotreesitter.Node, prefix, owner string) {
	for index := 0; index < container.NamedChildCount(); index++ {
		statement := container.NamedChild(index)
		switch w.typeOf(statement) {
		case typeFunction:
			w.function(statement, statement, prefix, owner)
		case typeClass:
			w.class(statement, prefix)
		case typeDecorated:
			definition := statement.ChildByFieldName(fieldDefinition, w.lang)
			switch {
			case definition == nil:
			case w.typeOf(definition) == typeFunction:
				w.function(statement, definition, prefix, owner)
			case w.typeOf(definition) == typeClass:
				w.class(definition, prefix)
			}
		case typeAssignment:
			if prefix == "" {
				w.assignment(statement, statement)
			}
		case typeExpression:
			if prefix == "" && statement.NamedChildCount() == singleNamedChildren && w.typeOf(statement.NamedChild(0)) == typeAssignment {
				w.assignment(statement, statement.NamedChild(0))
			}
		}
	}
}

func (w *walker) class(class *gotreesitter.Node, prefix string) {
	name := w.nameOf(class)
	body := class.ChildByFieldName(fieldBody, w.lang)
	if name == "" || body == nil {
		return
	}
	qualified := name
	if prefix != "" {
		qualified = fmt.Sprintf(qualifierFormat, prefix, name)
	}
	w.body(body, qualified, name)
}

func (w *walker) nameOf(node *gotreesitter.Node) string {
	if named := node.ChildByFieldName(fieldName, w.lang); named != nil {
		return w.text(named)
	}
	return ""
}

// function adds a function or method; span is the node that carries its
// decorators, definition the function itself.
func (w *walker) function(span, definition *gotreesitter.Node, prefix, owner string) {
	name := w.nameOf(definition)
	if name == "" {
		return
	}
	qualified := name
	if prefix != "" {
		qualified = fmt.Sprintf(qualifierFormat, prefix, name)
	}
	w.add(span, w.qualname(qualified), name, owner, code.KindFunction)
}

// assignment adds the unit of a module-level `name = value` statement; span is
// the statement, assigned the assignment inside it, which the grammar may or may
// not wrap in an expression statement.
func (w *walker) assignment(span, assigned *gotreesitter.Node) {
	left := assigned.ChildByFieldName(fieldLeft, w.lang)
	if left == nil || w.typeOf(left) != typeIdentifier {
		return
	}
	name := w.text(left)
	w.add(span, w.qualname(name), name, "", code.KindAssignment)
}

func (w *walker) add(span *gotreesitter.Node, qualname, name, owner string, kind code.Kind) {
	var normal strings.Builder
	w.normalize(span, &normal)
	mentioned := map[string]bool{}
	w.collectMentions(span, mentioned)
	names := make([]string, 0, len(mentioned))
	for mention := range mentioned {
		names = append(names, mention)
	}
	slices.Sort(names)
	w.units = append(w.units, code.Unit{
		File: w.file, Qualname: qualname, Name: name, Owner: owner, Kind: kind,
		Normalized: normal.String(), Text: w.text(span),
		StartLine: int(span.StartPoint().Row) + 1, EndLine: int(span.EndPoint().Row) + 1,
		Mentions: names,
	})
}

// normalize writes the syntax tree of a node as nested parentheses over its
// leaves, leaving out comments and docstrings.
func (w *walker) normalize(node *gotreesitter.Node, out *strings.Builder) {
	if w.typeOf(node) == typeComment {
		return
	}
	if node.ChildCount() == 0 {
		out.WriteString(w.text(node))
		out.WriteString(tokenSeparator)
		return
	}
	out.WriteString(structureOpen + w.typeOf(node) + tokenSeparator)
	for index := 0; index < node.ChildCount(); index++ {
		child := node.Child(index)
		if w.isDocstring(node, index) {
			continue
		}
		w.normalize(child, out)
	}
	out.WriteString(structureClose + tokenSeparator)
}

// isDocstring reports whether the child at the position is the docstring of a
// body: a string statement that comes first in a block.
func (w *walker) isDocstring(parent *gotreesitter.Node, index int) bool {
	if w.typeOf(parent) != typeBlock {
		return false
	}
	for before := 0; before < index; before++ {
		if w.typeOf(parent.Child(before)) != typeComment {
			return false
		}
	}
	child := parent.Child(index)
	if w.typeOf(child) == typeString {
		return true
	}
	return w.typeOf(child) == typeExpression && child.NamedChildCount() == singleNamedChildren && w.typeOf(child.NamedChild(0)) == typeString
}

// collectMentions records every identifier of a node, which covers calls,
// attribute names and parameter names alike.
func (w *walker) collectMentions(node *gotreesitter.Node, found map[string]bool) {
	if w.typeOf(node) == typeIdentifier {
		found[w.text(node)] = true
		return
	}
	for index := 0; index < node.ChildCount(); index++ {
		w.collectMentions(node.Child(index), found)
	}
}
