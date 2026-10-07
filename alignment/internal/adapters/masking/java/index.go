package java

import (
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/syntax"
	"github.com/RachidChabane/spec-proof/alignment/internal/platform/textedit"
)

// fileIndex is a parsed Java file with the edits that remove what a judge must
// not see: its comments, the annotations that name a test and the name patterns
// of parametrized tests.
type fileIndex struct {
	*syntax.Tree
	file     string
	removals []textedit.Edit
}

func indexFile(file string, source []byte) (*fileIndex, error) {
	tree, err := syntax.Parse(grammars.JavaLanguage(), source)
	if err != nil {
		return nil, err
	}
	index := &fileIndex{Tree: tree, file: file}
	tree.Walk(tree.Root(), func(node, _ *gotreesitter.Node, _ string) bool {
		switch tree.Type(node) {
		case typeLineComment, typeBlockComment:
			index.removals = append(index.removals, textedit.RemoveText(source, syntax.Span(node)))
			return false
		case typeAnnotation, typeMarkerAnnotation:
			if removal, ok := index.annotationRemoval(node); ok {
				index.removals = append(index.removals, removal)
			}
		}
		return true
	})
	return index, nil
}

// annotationRemoval is the edit that takes a hidden annotation out, or the name
// pattern out of the annotation of a parametrized or repeated test.
func (x *fileIndex) annotationRemoval(annotation *gotreesitter.Node) (textedit.Edit, bool) {
	name := x.annotationName(annotation)
	if slices.Contains(hiddenAnnotations, name) {
		return textedit.RemoveText(x.Source, syntax.Span(annotation)), true
	}
	if name != annotationParameterized && name != annotationRepeated {
		return textedit.Edit{}, false
	}
	return x.patternRemoval(x.Field(annotation, fieldArguments))
}

// patternRemoval takes the `name = "..."` element out of an argument list, and the
// parentheses with it when nothing else is in them.
func (x *fileIndex) patternRemoval(arguments *gotreesitter.Node) (textedit.Edit, bool) {
	elements := x.childrenOf(arguments)
	for position, element := range elements {
		if x.Type(element) != typeElementValuePair || x.Text(x.Field(element, fieldKey)) != elementName {
			continue
		}
		switch {
		case len(elements) == 1:
			return textedit.Edit{Span: syntax.Span(arguments)}, true
		case position == len(elements)-1:
			return textedit.Edit{Span: textedit.Span{Start: int(elements[position-1].EndByte()), End: int(element.EndByte())}}, true
		default:
			return textedit.Edit{Span: textedit.Span{Start: int(element.StartByte()), End: int(elements[position+1].StartByte())}}, true
		}
	}
	return textedit.Edit{}, false
}

func (x *fileIndex) isComment(node *gotreesitter.Node) bool {
	kind := x.Type(node)
	return kind == typeLineComment || kind == typeBlockComment
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

// packageName is the package the file declares, empty for the default package.
func (x *fileIndex) packageName() string {
	for _, child := range x.childrenOf(x.Root()) {
		if x.Type(child) == typePackage {
			for _, part := range x.Named(child) {
				if kind := x.Type(part); kind == typeScopedIdentifier || kind == typeIdentifier {
					return x.Text(part)
				}
			}
		}
	}
	return ""
}

// annotationsOf lists the annotations in the modifiers of a declaration.
func (x *fileIndex) annotationsOf(declaration *gotreesitter.Node) []*gotreesitter.Node {
	modifiers := x.FirstOfType(declaration, typeModifiers)
	var found []*gotreesitter.Node
	for _, child := range x.Named(modifiers) {
		if kind := x.Type(child); kind == typeAnnotation || kind == typeMarkerAnnotation {
			found = append(found, child)
		}
	}
	return found
}

// annotationName is the simple name of an annotation, without its package.
func (x *fileIndex) annotationName(annotation *gotreesitter.Node) string {
	name := x.Text(x.Field(annotation, fieldName))
	return name[strings.LastIndex(name, ".")+1:]
}

// findAnnotation returns the annotation of the declaration with the simple name.
func (x *fileIndex) findAnnotation(declaration *gotreesitter.Node, name string) *gotreesitter.Node {
	for _, annotation := range x.annotationsOf(declaration) {
		if x.annotationName(annotation) == name {
			return annotation
		}
	}
	return nil
}

// annotationText reads a text an annotation carries: its only argument, or the
// element with the key.
func (x *fileIndex) annotationText(annotation *gotreesitter.Node, key string) (string, bool) {
	arguments := x.Field(annotation, fieldArguments)
	if arguments == nil {
		return "", false
	}
	for _, argument := range x.childrenOf(arguments) {
		switch x.Type(argument) {
		case typeStringLiteral:
			if key == "" {
				return x.stringContent(argument), true
			}
		case typeElementValuePair:
			if x.Text(x.Field(argument, fieldKey)) == key {
				if value := x.Field(argument, fieldValue); x.Type(value) == typeStringLiteral {
					return x.stringContent(value), true
				}
			}
		}
	}
	return "", false
}

// stringContent is the text of a string literal without its quotes.
func (x *fileIndex) stringContent(literal *gotreesitter.Node) string {
	var content strings.Builder
	for _, part := range x.Named(literal) {
		content.WriteString(x.Text(part))
	}
	return content.String()
}

// modifierWords lists the keyword modifiers of a declaration, without annotations.
func (x *fileIndex) modifierWords(declaration *gotreesitter.Node) []string {
	modifiers := x.FirstOfType(declaration, typeModifiers)
	var words []string
	for _, child := range x.Children(modifiers) {
		if kind := x.Type(child); kind != typeAnnotation && kind != typeMarkerAnnotation {
			words = append(words, x.Text(child))
		}
	}
	return words
}

func (x *fileIndex) hasModifier(declaration *gotreesitter.Node, word string) bool {
	return slices.Contains(x.modifierWords(declaration), word)
}

// declaredNames lists the names a member of a class binds.
func (x *fileIndex) declaredNames(member *gotreesitter.Node) []string {
	switch x.Type(member) {
	case typeField:
		var names []string
		for _, declarator := range x.FieldChildren(member, fieldDeclarator) {
			names = append(names, x.Text(x.Field(declarator, fieldName)))
		}
		return names
	case typeMethod, typeClass, typeInterface, typeEnum, typeRecord, typeAnnotationType:
		if name := x.Field(member, fieldName); name != nil {
			return []string{x.Text(name)}
		}
	}
	return nil
}

// bodyOf is the body of a type declaration.
func (x *fileIndex) bodyOf(declaration *gotreesitter.Node) *gotreesitter.Node {
	return x.Field(declaration, fieldBody)
}

// membersNodes lists the members of the body of a type declaration; the members
// of an enum are those that follow its constants.
func (x *fileIndex) memberNodes(declaration *gotreesitter.Node) []*gotreesitter.Node {
	body := x.bodyOf(declaration)
	if x.Type(body) == typeEnumBody {
		body = x.FirstOfType(body, typeEnumDeclarations)
	}
	return x.childrenOf(body)
}
