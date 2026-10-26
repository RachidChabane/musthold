// Package kotlin indexes the units of Kotlin source files with the pure-Go
// tree-sitter runtime: functions, methods, constructors, initializers, and the
// top-level, object and companion constants. A JUnit 5 test function is a unit of
// kind code.KindTest that lists the labels a runner may report for it. The normal
// form of a unit is the stream of its syntax tree's leaves without comments, so that
// a change of formatting, of a comment or of a KDoc is no change.
package kotlin

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/RachidChabane/musthold/completeness/internal/app/ports"
	"github.com/RachidChabane/musthold/completeness/internal/domain/code"
	"github.com/RachidChabane/musthold/completeness/internal/domain/testlabel"
	"github.com/RachidChabane/musthold/completeness/internal/platform/treeunits"
)

// Naming rules of Kotlin source files.
const (
	sourceExtension = ".kt"
	initName        = "<init>"
	companionName   = "Companion"
	testSuffix      = "Test"
	testsSuffix     = "Tests"
	integrationTest = "IT"
	testPrefix      = "Test"
	testCaseSuffix  = "TestCase"
	backtick        = "`"
)

// testDirectories are the directory names whose files hold tests.
var testDirectories = []string{"test", "tests", "testFixtures", "integrationTest", "it"}

// Node types of the tree-sitter Kotlin grammar.
const (
	typeAnnotation        = "annotation"
	typeAnonymousInit     = "anonymous_initializer"
	typeClass             = "class_declaration"
	typeClassBody         = "class_body"
	typeCompanion         = "companion_object"
	typeConstructorCall   = "constructor_invocation"
	typeError             = "ERROR"
	typeEnumBody          = "enum_class_body"
	typeEnumEntry         = "enum_entry"
	typeFunction          = "function_declaration"
	typeInterpolatedName  = "interpolated_identifier"
	typeLineComment       = "line_comment"
	typeModifiers         = "modifiers"
	typeMultiVariable     = "multi_variable_declaration"
	typeMultilineComment  = "multiline_comment"
	typeObject            = "object_declaration"
	typePrimaryConstuctor = "primary_constructor"
	typeProperty          = "property_declaration"
	typeSecondaryCtor     = "secondary_constructor"
	typeSimpleIdentifier  = "simple_identifier"
	typeStringContent     = "string_content"
	typeStringLiteral     = "string_literal"
	typeTypeIdentifier    = "type_identifier"
	typeUserType          = "user_type"
	typeValueArguments    = "value_arguments"
	typeVariableDecl      = "variable_declaration"
	tokenAssign           = "="
	elementName           = "name"
)

// Annotations of JUnit 5 by their simple names.
const (
	annotationTest          = "Test"
	annotationParameterized = "ParameterizedTest"
	annotationRepeated      = "RepeatedTest"
	annotationFactory       = "TestFactory"
	annotationTemplate      = "TestTemplate"
	annotationDisplayName   = "DisplayName"
)

var (
	testAnnotations  = []string{annotationTest, annotationParameterized, annotationRepeated, annotationFactory, annotationTemplate}
	setupAnnotations = []string{"BeforeEach", "BeforeAll", "AfterEach", "AfterAll", "RegisterExtension"}
)

// rules tell the comments and the identifiers of the grammar.
var rules = treeunits.Rules{
	Comments:    []string{typeLineComment, typeMultilineComment},
	Identifiers: []string{typeSimpleIdentifier, typeTypeIdentifier, typeInterpolatedName},
}

var _ ports.Indexer = Indexer{}

// Indexer implements ports.Indexer for Kotlin.
type Indexer struct{}

// New returns the Kotlin indexer.
func New() Indexer { return Indexer{} }

// Handles reports whether the file is a Kotlin source file.
func (Indexer) Handles(file string) bool { return path.Ext(file) == sourceExtension }

// IsTest reports whether the file holds Kotlin tests: a file below a `test`
// directory, as `src/test/kotlin` of Maven and Gradle, or one named `*Test`,
// `*Tests`, `*IT`, `*TestCase` or `Test*`.
func (Indexer) IsTest(file string) bool {
	if slices.ContainsFunc(strings.Split(path.Dir(file), "/"), func(segment string) bool { return slices.Contains(testDirectories, segment) }) {
		return true
	}
	stem := strings.TrimSuffix(path.Base(file), sourceExtension)
	for _, suffix := range []string{testSuffix, testsSuffix, integrationTest, testCaseSuffix} {
		if strings.HasSuffix(stem, suffix) && stem != suffix {
			return true
		}
	}
	return strings.HasPrefix(stem, testPrefix) && stem != testPrefix
}

// Index parses the source and lists its units in source order. The grammar recovers
// from syntax errors, so a broken file still yields the units it could read.
func (i Indexer) Index(file string, source []byte) (code.File, error) {
	tree, err := treeunits.ParseRewritten(grammars.KotlinLanguage(), groupedAnnotations(source), source)
	if err != nil {
		return code.File{}, fmt.Errorf("parse %s: %w", file, err)
	}
	defer tree.Release()
	walk := &walker{tree: tree, units: treeunits.NewCollector(tree, file, rules)}
	for _, declaration := range walk.children(tree.Root()) {
		walk.topLevel(declaration)
	}
	return code.File{Path: file, Test: i.IsTest(file), Units: walk.units.Units()}, nil
}

type walker struct {
	tree  *treeunits.Tree
	units *treeunits.Collector
}

// class is a type of the chain a member is declared in.
type class struct {
	name, display string
}

func (w *walker) children(node *gotreesitter.Node) []*gotreesitter.Node {
	var children []*gotreesitter.Node
	for _, child := range w.tree.Named(node) {
		switch kind := w.tree.Type(child); kind {
		case typeLineComment, typeMultilineComment:
		case typeError:
			children = append(children, w.children(child)...)
		default:
			children = append(children, child)
		}
	}
	return children
}

func (w *walker) isType(node *gotreesitter.Node) bool {
	kind := w.tree.Type(node)
	return kind == typeClass || kind == typeObject
}

// topLevel lists the units a top-level declaration of the file declares.
func (w *walker) topLevel(declaration *gotreesitter.Node) {
	switch w.tree.Type(declaration) {
	case typeFunction:
		name := w.functionName(declaration)
		w.units.Add(treeunits.Spec{Node: declaration, Qualname: name, Name: name, Kind: code.KindFunction})
	case typeProperty:
		w.properties(declaration, nil)
	case typeClass, typeObject:
		w.declare(declaration, nil, nil)
	}
}

func (w *walker) functionName(function *gotreesitter.Node) string {
	return strings.Trim(w.tree.Text(w.tree.FirstOfType(function, typeSimpleIdentifier)), backtick)
}

func (w *walker) typeName(declaration *gotreesitter.Node) string {
	return w.tree.Text(w.tree.FirstOfType(declaration, typeTypeIdentifier))
}

func (w *walker) bodyOf(declaration *gotreesitter.Node) *gotreesitter.Node {
	return w.tree.FirstOfType(declaration, typeClassBody, typeEnumBody)
}

// propertyNames lists the names a property declaration binds.
func (w *walker) propertyNames(property *gotreesitter.Node) []string {
	var names []string
	for _, child := range w.tree.Named(property) {
		switch w.tree.Type(child) {
		case typeVariableDecl:
			names = append(names, w.tree.Text(w.tree.FirstOfType(child, typeSimpleIdentifier)))
		case typeMultiVariable:
			for _, part := range w.tree.Named(child) {
				names = append(names, w.tree.Text(w.tree.FirstOfType(part, typeSimpleIdentifier)))
			}
		}
	}
	return names
}

// properties adds a unit per name a property declaration binds; a declaration of
// several names gives the declaration as the text of each.
func (w *walker) properties(property *gotreesitter.Node, chain []class) {
	owner := ""
	if len(chain) > 0 {
		owner = chain[len(chain)-1].name
	}
	for _, name := range w.propertyNames(property) {
		w.units.Add(treeunits.Spec{Node: property, Qualname: w.qualified(chain, name), Name: name, Owner: owner, Kind: code.KindAssignment})
	}
}

func (w *walker) qualified(chain []class, name string) string {
	parts := make([]string, 0, len(chain)+1)
	for _, c := range chain {
		parts = append(parts, c.name)
	}
	return strings.Join(append(parts, name), ".")
}

// declare lists the units of a class, an interface or an object and of the types
// nested in it. chain holds the types around it and setup what runs for every test
// of them.
func (w *walker) declare(declaration *gotreesitter.Node, chain []class, setup []*gotreesitter.Node) {
	owner := class{name: w.typeName(declaration), display: w.displayName(declaration)}
	inner := append(slices.Clone(chain), owner)
	members := w.children(w.bodyOf(declaration))
	around := slices.Clone(setup)
	for _, member := range members {
		if w.runsForEveryTest(member) {
			around = append(around, member)
		}
	}
	if constructor := w.tree.FirstOfType(declaration, typePrimaryConstuctor); constructor != nil {
		w.units.Add(treeunits.Spec{Node: constructor, Qualname: w.qualified(inner, initName), Name: owner.name, Owner: owner.name, Kind: code.KindFunction})
	}
	holdsConstants := w.tree.Type(declaration) == typeObject
	for _, member := range members {
		w.member(member, inner, around, holdsConstants)
	}
}

func (w *walker) member(member *gotreesitter.Node, chain []class, setup []*gotreesitter.Node, holdsConstants bool) {
	owner := chain[len(chain)-1].name
	switch w.tree.Type(member) {
	case typeFunction:
		w.function(member, chain, owner, setup)
	case typeSecondaryCtor, typeAnonymousInit:
		w.units.Add(treeunits.Spec{Node: member, Qualname: w.qualified(chain, initName), Name: owner, Owner: owner, Kind: code.KindFunction})
	case typeProperty:
		if holdsConstants {
			w.properties(member, chain)
		}
	case typeEnumEntry:
		name := w.tree.Text(w.tree.FirstOfType(member, typeSimpleIdentifier))
		w.units.Add(treeunits.Spec{Node: member, Qualname: w.qualified(chain, name), Name: name, Owner: owner, Kind: code.KindAssignment})
	case typeCompanion:
		w.companion(member, chain, setup)
	case typeClass, typeObject:
		w.declare(member, chain, setup)
	}
}

// companion lists the units of a companion object, which are the constants and the
// functions of the class that holds it.
func (w *walker) companion(companion *gotreesitter.Node, chain []class, setup []*gotreesitter.Node) {
	inner := append(slices.Clone(chain), class{name: companionName})
	owner := chain[len(chain)-1].name
	for _, member := range w.children(w.bodyOf(companion)) {
		switch w.tree.Type(member) {
		case typeFunction:
			w.function(member, inner, owner, setup)
		case typeProperty:
			for _, name := range w.propertyNames(member) {
				w.units.Add(treeunits.Spec{Node: member, Qualname: w.qualified(inner, name), Name: name, Owner: owner, Kind: code.KindAssignment})
			}
		}
	}
}

// function adds the unit of a function: a test when a JUnit 5 annotation says so, a
// function otherwise.
func (w *walker) function(function *gotreesitter.Node, chain []class, owner string, setup []*gotreesitter.Node) {
	name := w.functionName(function)
	spec := treeunits.Spec{Node: function, Qualname: w.qualified(chain, name), Name: name, Owner: owner, Kind: code.KindFunction}
	if w.hasAnnotation(function, testAnnotations...) {
		spec.Kind, spec.Also, spec.Labels = code.KindTest, setup, []string{testlabel.JUnitPattern(w.testOf(function, name, chain))}
	}
	w.units.Add(spec)
}

// runsForEveryTest reports whether a member of a class runs for each test of it: a
// hook, an extension or an initializer.
func (w *walker) runsForEveryTest(member *gotreesitter.Node) bool {
	switch w.tree.Type(member) {
	case typeAnonymousInit:
		return true
	case typeFunction, typeProperty:
		return w.hasAnnotation(member, setupAnnotations...)
	}
	return false
}

// annotation is one annotation of a declaration.
type annotation struct {
	item *gotreesitter.Node
	name string
}

// annotations lists the annotations in the modifiers of a declaration.
func (w *walker) annotations(declaration *gotreesitter.Node) []annotation {
	var found []annotation
	for _, node := range w.tree.Named(w.tree.FirstOfType(declaration, typeModifiers)) {
		if w.tree.Type(node) != typeAnnotation {
			continue
		}
		for _, item := range w.tree.Named(node) {
			if kind := w.tree.Type(item); kind == typeConstructorCall || kind == typeUserType {
				found = append(found, annotation{item: item, name: w.annotationName(item)})
			}
		}
	}
	return found
}

func (w *walker) annotationName(item *gotreesitter.Node) string {
	user := item
	if w.tree.Type(item) == typeConstructorCall {
		user = w.tree.FirstOfType(item, typeUserType)
	}
	name := w.tree.Text(user)
	return name[strings.LastIndex(name, ".")+1:]
}

func (w *walker) hasAnnotation(declaration *gotreesitter.Node, names ...string) bool {
	return slices.ContainsFunc(w.annotations(declaration), func(a annotation) bool { return slices.Contains(names, a.name) })
}

// argumentKey is the name of a named argument, empty for a positional one.
func (w *walker) argumentKey(argument *gotreesitter.Node) string {
	children := w.tree.Children(argument)
	if len(children) >= 3 && w.tree.Text(children[1]) == tokenAssign && w.tree.Type(children[0]) == typeSimpleIdentifier {
		return w.tree.Text(children[0])
	}
	return ""
}

// annotationText reads a text an annotation carries: its only positional argument,
// or the named argument with the key.
func (w *walker) annotationText(a annotation, key string) string {
	if w.tree.Type(a.item) != typeConstructorCall {
		return ""
	}
	for _, argument := range w.tree.Named(w.tree.FirstOfType(a.item, typeValueArguments)) {
		if w.argumentKey(argument) != key {
			continue
		}
		if literal := w.tree.FirstOfType(argument, typeStringLiteral); literal != nil {
			var content strings.Builder
			for _, part := range w.tree.Named(literal) {
				if w.tree.Type(part) == typeStringContent {
					content.WriteString(w.tree.Text(part))
				}
			}
			return content.String()
		}
	}
	return ""
}

// displayName is the display name of a type, empty when it has none.
func (w *walker) displayName(declaration *gotreesitter.Node) string {
	for _, found := range w.annotations(declaration) {
		if found.name == annotationDisplayName {
			return w.annotationText(found, "")
		}
	}
	return ""
}

// testOf reads what a runner reports about a test function from its annotations.
func (w *walker) testOf(function *gotreesitter.Node, name string, chain []class) testlabel.JUnitTest {
	test := testlabel.JUnitTest{Method: name}
	for _, c := range chain {
		test.Classes = append(test.Classes, testlabel.JUnitClass{Name: c.name, Display: c.display})
	}
	for _, found := range w.annotations(function) {
		switch found.name {
		case annotationDisplayName:
			test.Display = w.annotationText(found, "")
		case annotationParameterized:
			test.Parameterized = true
			test.Template = w.annotationText(found, elementName)
		case annotationRepeated:
			test.Repeated = true
			test.Template = w.annotationText(found, elementName)
		}
	}
	return test
}
