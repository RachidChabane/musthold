package kotlin

import (
	"fmt"
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/alignment/internal/platform/junit"
)

// memberKind tells what a member of a class is for the masker.
type memberKind int

const (
	memberOther memberKind = iota
	// memberDeclaration binds a name a test may use: a property, a function, a type.
	memberDeclaration
	// memberSetup runs for every test of its class: a hook, an extension, an
	// initializer.
	memberSetup
	// memberTest is a test function.
	memberTest
)

// member is a member of a class body.
type member struct {
	kind  memberKind
	node  *gotreesitter.Node
	names []string
}

// classScope is a class that may hold tests, with its members.
type classScope struct {
	node    *gotreesitter.Node
	name    string
	display string
	members []member
}

// testCase is a test function with the classes it is declared in, outermost first.
type testCase struct {
	function *gotreesitter.Node
	chain    []*classScope
	info     junit.Test
}

// isType reports whether the node declares a class, an interface or an object.
func (x *fileIndex) isType(node *gotreesitter.Node) bool {
	kind := x.Type(node)
	return kind == typeClass || kind == typeObject
}

// topLevelTypes lists the type declarations of the file.
func (x *fileIndex) topLevelTypes() []*gotreesitter.Node {
	var found []*gotreesitter.Node
	for _, child := range x.childrenOf(x.Root()) {
		if x.isType(child) {
			found = append(found, child)
		}
	}
	return found
}

// typeName is the name of a class, interface or object.
func (x *fileIndex) typeName(declaration *gotreesitter.Node) string {
	return x.Text(x.childOfType(declaration, typeTypeIdentifier))
}

// bodyOf is the body of a type declaration.
func (x *fileIndex) bodyOf(declaration *gotreesitter.Node) *gotreesitter.Node {
	return x.childOfType(declaration, typeClassBody, typeEnumBody)
}

// functionName is the name of a function without the backticks of a sentence.
func (x *fileIndex) functionName(function *gotreesitter.Node) string {
	return strings.Trim(x.Text(x.childOfType(function, typeSimpleIdentifier)), tokenBacktick)
}

// propertyNames lists the names a property declaration binds.
func (x *fileIndex) propertyNames(property *gotreesitter.Node) []string {
	var names []string
	for _, child := range x.Named(property) {
		switch x.Type(child) {
		case typeVariableDeclaration:
			names = append(names, x.Text(x.childOfType(child, typeSimpleIdentifier)))
		case typeMultiVariable:
			for _, part := range x.Named(child) {
				names = append(names, x.Text(x.childOfType(part, typeSimpleIdentifier)))
			}
		}
	}
	return names
}

// declaredNames lists the names a member of a class binds.
func (x *fileIndex) declaredNames(member *gotreesitter.Node) []string {
	switch x.Type(member) {
	case typeFunction:
		return []string{x.functionName(member)}
	case typeProperty:
		return x.propertyNames(member)
	case typeClass, typeObject:
		return []string{x.typeName(member)}
	case typeCompanion:
		var names []string
		for _, inner := range x.childrenOf(x.bodyOf(member)) {
			names = append(names, x.declaredNames(inner)...)
		}
		return names
	}
	return nil
}

func (x *fileIndex) scopeOf(class *gotreesitter.Node) *classScope {
	scope := &classScope{node: class, name: x.typeName(class)}
	if found, ok := x.find(class, annotationDisplayName); ok {
		scope.display, _ = x.annotationText(found, "")
	}
	for _, node := range x.childrenOf(x.bodyOf(class)) {
		scope.members = append(scope.members, x.memberOf(node))
	}
	return scope
}

func (x *fileIndex) memberOf(node *gotreesitter.Node) member {
	switch x.Type(node) {
	case typeAnonymousInit:
		return member{kind: memberSetup, node: node}
	case typeFunction:
		if x.isTest(node) {
			return member{kind: memberTest, node: node, names: x.declaredNames(node)}
		}
		if x.isSetup(node) {
			return member{kind: memberSetup, node: node, names: x.declaredNames(node)}
		}
	case typeProperty:
		if x.isSetup(node) {
			return member{kind: memberSetup, node: node, names: x.declaredNames(node)}
		}
	}
	if names := x.declaredNames(node); len(names) > 0 {
		return member{kind: memberDeclaration, node: node, names: names}
	}
	return member{kind: memberOther, node: node}
}

var testAnnotations = []string{annotationTest, annotationParameterized, annotationRepeated, annotationFactory, annotationTemplate}

func (x *fileIndex) isTest(function *gotreesitter.Node) bool {
	return slices.ContainsFunc(x.annotationsOf(function), func(a annotation) bool { return slices.Contains(testAnnotations, a.name) })
}

func (x *fileIndex) isSetup(declaration *gotreesitter.Node) bool {
	return slices.ContainsFunc(x.annotationsOf(declaration), func(a annotation) bool { return slices.Contains(setupAnnotations, a.name) })
}

// testCases lists every test function of the file with its classes.
func (x *fileIndex) testCases() []testCase {
	var found []testCase
	for _, class := range x.topLevelTypes() {
		x.collect(x.scopeOf(class), nil, &found)
	}
	return found
}

func (x *fileIndex) collect(scope *classScope, path []*classScope, out *[]testCase) {
	chain := append(slices.Clone(path), scope)
	for _, m := range scope.members {
		switch {
		case m.kind == memberTest:
			*out = append(*out, testCase{function: m.node, chain: chain, info: x.testInfo(m.node, chain)})
		case x.isType(m.node):
			x.collect(x.scopeOf(m.node), chain, out)
		}
	}
}

// testInfo reads what a runner reports about a test function from its annotations.
func (x *fileIndex) testInfo(function *gotreesitter.Node, chain []*classScope) junit.Test {
	info := junit.Test{
		Method:    x.functionName(function),
		StartLine: x.line(int(function.StartByte())), EndLine: x.line(int(function.EndByte())),
	}
	for _, scope := range chain {
		info.Classes = append(info.Classes, junit.Class{Name: scope.name, Display: scope.display, Line: x.line(int(scope.node.StartByte()))})
	}
	for _, found := range x.annotationsOf(function) {
		switch found.name {
		case annotationDisplayName:
			info.Display, _ = x.annotationText(found, "")
		case annotationParameterized:
			info.Kind = junit.Parameterized
			info.Template, _ = x.annotationText(found, elementName)
		case annotationRepeated:
			info.Kind = junit.Repeated
			info.Template, _ = x.annotationText(found, elementName)
		case annotationFactory, annotationTemplate:
			info.Kind = junit.Dynamic
		}
	}
	return info
}

// locate finds the test function a report names.
func (x *fileIndex) locate(bound criterion.BoundTest) (testCase, error) {
	candidates := x.testCases()
	infos := make([]junit.Test, len(candidates))
	for index, candidate := range candidates {
		infos[index] = candidate.info
	}
	index, found := junit.Locate(infos, bound.Name, bound.Classname, bound.Line)
	if !found {
		return testCase{}, fmt.Errorf("%w: %s in %s", ports.ErrTestNotFound, bound.Label(), bound.File)
	}
	return candidates[index], nil
}
