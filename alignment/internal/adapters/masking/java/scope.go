package java

import (
	"fmt"
	"slices"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/platform/junit"
	"github.com/RachidChabane/musthold/alignment/internal/platform/syntax"
)

// memberKind tells what a member of a class is for the masker.
type memberKind int

const (
	memberOther memberKind = iota
	// memberDeclaration binds a name a test may use: a field, a method, a type.
	memberDeclaration
	// memberSetup runs for every test of its class: a hook, an extension, an
	// initializer.
	memberSetup
	// memberTest is a test method.
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

// testCase is a test method with the classes it is declared in, outermost first.
type testCase struct {
	method *gotreesitter.Node
	chain  []*classScope
	info   junit.Test
}

// topLevelTypes lists the type declarations of the file.
func (x *fileIndex) topLevelTypes() []*gotreesitter.Node {
	var found []*gotreesitter.Node
	for _, child := range x.childrenOf(x.Root()) {
		switch x.Type(child) {
		case typeClass, typeInterface, typeEnum, typeRecord, typeAnnotationType:
			found = append(found, child)
		}
	}
	return found
}

func (x *fileIndex) scopeOf(class *gotreesitter.Node) *classScope {
	scope := &classScope{node: class, name: x.Text(x.Field(class, fieldName))}
	if annotation := x.findAnnotation(class, annotationDisplayName); annotation != nil {
		scope.display, _ = x.annotationText(annotation, "")
	}
	for _, node := range x.memberNodes(class) {
		scope.members = append(scope.members, x.memberOf(node))
	}
	return scope
}

func (x *fileIndex) memberOf(node *gotreesitter.Node) member {
	switch x.Type(node) {
	case typeStaticInitializer, typeBlock:
		return member{kind: memberSetup, node: node}
	case typeMethod:
		if x.isTest(node) {
			return member{kind: memberTest, node: node, names: x.declaredNames(node)}
		}
		if x.isSetup(node) {
			return member{kind: memberSetup, node: node, names: x.declaredNames(node)}
		}
	case typeField:
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

func (x *fileIndex) isTest(method *gotreesitter.Node) bool {
	return slices.ContainsFunc(x.annotationsOf(method), func(a *gotreesitter.Node) bool {
		return slices.Contains(testAnnotations, x.annotationName(a))
	})
}

func (x *fileIndex) isSetup(declaration *gotreesitter.Node) bool {
	return slices.ContainsFunc(x.annotationsOf(declaration), func(a *gotreesitter.Node) bool {
		return slices.Contains(setupAnnotations, x.annotationName(a))
	})
}

// testCases lists every test method of the file with its classes.
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
			*out = append(*out, testCase{method: m.node, chain: chain, info: x.testInfo(m.node, chain)})
		case slices.Contains([]string{typeClass, typeInterface, typeEnum, typeRecord}, x.Type(m.node)):
			x.collect(x.scopeOf(m.node), chain, out)
		}
	}
}

// testInfo reads what a runner reports about a test method from its annotations.
func (x *fileIndex) testInfo(method *gotreesitter.Node, chain []*classScope) junit.Test {
	info := junit.Test{
		Method:    x.Text(x.Field(method, fieldName)),
		StartLine: syntax.Line(method), EndLine: syntax.EndLine(method),
	}
	for _, scope := range chain {
		info.Classes = append(info.Classes, junit.Class{Name: scope.name, Display: scope.display, Line: syntax.Line(scope.node)})
	}
	for _, annotation := range x.annotationsOf(method) {
		switch x.annotationName(annotation) {
		case annotationDisplayName:
			info.Display, _ = x.annotationText(annotation, "")
		case annotationParameterized:
			info.Kind = junit.Parameterized
			info.Template, _ = x.annotationText(annotation, elementName)
		case annotationRepeated:
			info.Kind = junit.Repeated
			info.Template, _ = x.annotationText(annotation, elementName)
		case annotationFactory, annotationTemplate:
			info.Kind = junit.Dynamic
		}
	}
	return info
}

// locate finds the test method a report names.
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
