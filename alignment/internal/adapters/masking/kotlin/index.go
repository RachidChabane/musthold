package kotlin

import (
	"bytes"
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/syntax"
	"github.com/RachidChabane/spec-proof/alignment/internal/platform/textedit"
)

// fileIndex is a parsed Kotlin file with the edits that remove what a judge must
// not see: its comments, the annotations that name a test and the name patterns of
// parametrized tests.
type fileIndex struct {
	*syntax.Tree
	file string
	// original is the source as the project holds it; the tree is built on the
	// source with its grouped annotations rewritten.
	original []byte
	removals []textedit.Edit
}

func indexFile(file string, source []byte) (*fileIndex, error) {
	tree, err := syntax.Parse(grammars.KotlinLanguage(), groupedAnnotations(source))
	if err != nil {
		return nil, err
	}
	index := &fileIndex{Tree: tree, file: file, original: source}
	tree.Walk(tree.Root(), func(node, parent *gotreesitter.Node, _ string) bool {
		switch tree.Type(node) {
		case typeLineComment, typeMultilineComment:
			index.removals = append(index.removals, textedit.RemoveText(tree.Source, syntax.Span(node)))
			return false
		case typeAnnotation:
			if removal, ok := index.annotationRemoval(node, parent); ok {
				index.removals = append(index.removals, removal)
			}
		}
		return true
	})
	return index, nil
}

// line is the 1-based line of the offset in the source the project holds.
func (x *fileIndex) line(offset int) int { return 1 + bytes.Count(x.original[:offset], []byte("\n")) }

func (x *fileIndex) isComment(node *gotreesitter.Node) bool {
	kind := x.Type(node)
	return kind == typeLineComment || kind == typeMultilineComment
}

// childrenOf lists the named children of a node that are not comments.
func (x *fileIndex) childrenOf(node *gotreesitter.Node) []*gotreesitter.Node {
	var children []*gotreesitter.Node
	for _, child := range x.Named(node) {
		if !x.isComment(child) {
			children = append(children, child)
		}
	}
	return children
}

// childOfType is the first named child with the grammar type.
func (x *fileIndex) childOfType(node *gotreesitter.Node, kinds ...string) *gotreesitter.Node {
	for _, child := range x.Named(node) {
		if slices.Contains(kinds, x.Type(child)) {
			return child
		}
	}
	return nil
}

// packageName is the package the file declares, empty for the default package.
func (x *fileIndex) packageName() string {
	header := x.childOfType(x.Root(), typePackageHeader)
	return x.Text(x.childOfType(header, typeIdentifier))
}

// annotation is one annotation of a declaration.
type annotation struct {
	// node is the annotation node, which holds several items when the annotations
	// are grouped; item is the one this annotation is.
	node, item *gotreesitter.Node
	name       string
}

// annotationsOf lists the annotations in the modifiers of a declaration.
func (x *fileIndex) annotationsOf(declaration *gotreesitter.Node) []annotation {
	var found []annotation
	for _, node := range x.Named(x.childOfType(declaration, typeModifiers)) {
		if x.Type(node) != typeAnnotation {
			continue
		}
		for _, item := range x.Named(node) {
			switch x.Type(item) {
			case typeConstructorCall, typeUserType:
				found = append(found, annotation{node: node, item: item, name: x.annotationName(item)})
			}
		}
	}
	return found
}

// annotationName is the simple name of an annotation, without its package.
func (x *fileIndex) annotationName(item *gotreesitter.Node) string {
	user := item
	if x.Type(item) == typeConstructorCall {
		user = x.childOfType(item, typeUserType)
	}
	name := x.Text(user)
	return name[strings.LastIndex(name, ".")+1:]
}

// find returns the annotation of the declaration with the simple name.
func (x *fileIndex) find(declaration *gotreesitter.Node, name string) (annotation, bool) {
	for _, candidate := range x.annotationsOf(declaration) {
		if candidate.name == name {
			return candidate, true
		}
	}
	return annotation{}, false
}

// arguments lists the arguments of an annotation.
func (x *fileIndex) arguments(a annotation) []*gotreesitter.Node {
	if x.Type(a.item) != typeConstructorCall {
		return nil
	}
	return x.Named(x.childOfType(a.item, typeValueArguments))
}

// argumentKey is the name of a named argument, empty for a positional one.
func (x *fileIndex) argumentKey(argument *gotreesitter.Node) string {
	children := x.Children(argument)
	if len(children) >= 3 && x.Text(children[1]) == tokenAssign && x.Type(children[0]) == typeSimpleIdentifier {
		return x.Text(children[0])
	}
	return ""
}

// annotationText reads a text an annotation carries: its only positional argument,
// or the named argument with the key.
func (x *fileIndex) annotationText(a annotation, key string) (string, bool) {
	for _, argument := range x.arguments(a) {
		if x.argumentKey(argument) != key {
			continue
		}
		if literal := x.childOfType(argument, typeStringLiteral); literal != nil {
			return x.stringContent(literal), true
		}
	}
	return "", false
}

// stringContent is the text of a string literal without its quotes.
func (x *fileIndex) stringContent(literal *gotreesitter.Node) string {
	var content strings.Builder
	for _, part := range x.Named(literal) {
		if x.Type(part) == typeStringContent {
			content.WriteString(x.Text(part))
		}
	}
	return content.String()
}

// annotationRemoval is the edit that takes a hidden annotation out, or the name
// pattern out of the annotation of a parametrized or repeated test. Annotations
// written as a group are read item by item and need no edit, because the class
// they belong to is not rendered.
func (x *fileIndex) annotationRemoval(node, parent *gotreesitter.Node) (textedit.Edit, bool) {
	if x.Type(parent) != typeModifiers || len(x.annotationItems(node)) != 1 {
		return textedit.Edit{}, false
	}
	item := x.annotationItems(node)[0]
	name := x.annotationName(item)
	if slices.Contains(hiddenAnnotations, name) {
		return textedit.RemoveText(x.Source, syntax.Span(node)), true
	}
	if name != annotationParameterized && name != annotationRepeated {
		return textedit.Edit{}, false
	}
	return x.patternRemoval(annotation{node: node, item: item, name: name})
}

func (x *fileIndex) annotationItems(node *gotreesitter.Node) []*gotreesitter.Node {
	var items []*gotreesitter.Node
	for _, item := range x.Named(node) {
		if kind := x.Type(item); kind == typeConstructorCall || kind == typeUserType {
			items = append(items, item)
		}
	}
	return items
}

// patternRemoval takes the `name = "..."` argument out of an argument list, and the
// parentheses with it when nothing else is in them.
func (x *fileIndex) patternRemoval(a annotation) (textedit.Edit, bool) {
	arguments := x.arguments(a)
	list := x.childOfType(a.item, typeValueArguments)
	for position, argument := range arguments {
		if x.argumentKey(argument) != elementName {
			continue
		}
		switch {
		case len(arguments) == 1:
			return textedit.Edit{Span: syntax.Span(list)}, true
		case position == len(arguments)-1:
			return textedit.Edit{Span: textedit.Span{Start: int(arguments[position-1].EndByte()), End: int(argument.EndByte())}}, true
		default:
			return textedit.Edit{Span: textedit.Span{Start: int(argument.StartByte()), End: int(arguments[position+1].StartByte())}}, true
		}
	}
	return textedit.Edit{}, false
}
