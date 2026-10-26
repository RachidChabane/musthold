// Package java indexes the units of Java source files with the pure-Go tree-sitter
// runtime: methods, constructors, static initializers, and the constants of a class
// or an enumeration. A JUnit 5 test method is a unit of kind code.KindTest that lists
// the labels a runner may report for it. The normal form of a unit is the stream of
// its syntax tree's leaves without comments, so that a change of formatting, of a
// comment or of a Javadoc is no change.
package java

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

// Naming rules of Java source files.
const (
	sourceExtension = ".java"
	qualifierFormat = "%s.%s"
	initName        = "<init>"
	classInitName   = "<clinit>"
	testSuffix      = "Test"
	testsSuffix     = "Tests"
	integrationTest = "IT"
	testPrefix      = "Test"
	testCaseSuffix  = "TestCase"
)

// testDirectories are the directory names whose files hold tests.
var testDirectories = []string{"test", "tests", "testFixtures", "integrationTest", "it"}

// Node types and fields of the tree-sitter Java grammar.
const (
	typeAnnotation       = "annotation"
	typeBlock            = "block"
	typeBlockComment     = "block_comment"
	typeClass            = "class_declaration"
	typeCompactCtor      = "compact_constructor_declaration"
	typeConstant         = "constant_declaration"
	typeConstructor      = "constructor_declaration"
	typeError            = "ERROR"
	typeEnum             = "enum_declaration"
	typeEnumBody         = "enum_body"
	typeEnumConstant     = "enum_constant"
	typeEnumDeclarations = "enum_body_declarations"
	typeField            = "field_declaration"
	typeIdentifier       = "identifier"
	typeInterface        = "interface_declaration"
	typeLineComment      = "line_comment"
	typeMarkerAnnotation = "marker_annotation"
	typeMethod           = "method_declaration"
	typeModifiers        = "modifiers"
	typeRecord           = "record_declaration"
	typeAnnotationType   = "annotation_type_declaration"
	typeStaticInit       = "static_initializer"
	typeStringLiteral    = "string_literal"
	typeTypeIdentifier   = "type_identifier"
	typeElementPair      = "element_value_pair"
	fieldArguments       = "arguments"
	fieldBody            = "body"
	fieldDeclarator      = "declarator"
	fieldKey             = "key"
	fieldName            = "name"
	fieldValue           = "value"
	tokenStatic          = "static"
	tokenFinal           = "final"
	elementName          = "name"
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
	Comments:    []string{typeLineComment, typeBlockComment},
	Identifiers: []string{typeIdentifier, typeTypeIdentifier},
}

var _ ports.Indexer = Indexer{}

// Indexer implements ports.Indexer for Java.
type Indexer struct{}

// New returns the Java indexer.
func New() Indexer { return Indexer{} }

// Handles reports whether the file is a Java source file.
func (Indexer) Handles(file string) bool { return path.Ext(file) == sourceExtension }

// IsTest reports whether the file holds Java tests: a file below a `test` directory,
// as `src/test/java` of Maven and Gradle, or one named `*Test`, `*Tests`, `*IT`,
// `*TestCase` or `Test*`.
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
	tree, err := treeunits.Parse(grammars.JavaLanguage(), source)
	if err != nil {
		return code.File{}, fmt.Errorf("parse %s: %w", file, err)
	}
	defer tree.Release()
	walk := &walker{tree: tree, units: treeunits.NewCollector(tree, file, rules)}
	for _, declaration := range walk.typesIn(walk.children(tree.Root())) {
		walk.declare(declaration, nil, nil)
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
		case typeLineComment, typeBlockComment:
		case typeError:
			children = append(children, w.children(child)...)
		default:
			children = append(children, child)
		}
	}
	return children
}

// typesIn lists the type declarations among the nodes.
func (w *walker) typesIn(nodes []*gotreesitter.Node) []*gotreesitter.Node {
	var found []*gotreesitter.Node
	for _, node := range nodes {
		if w.isType(node) {
			found = append(found, node)
		}
	}
	return found
}

func (w *walker) isType(node *gotreesitter.Node) bool {
	switch w.tree.Type(node) {
	case typeClass, typeInterface, typeEnum, typeRecord, typeAnnotationType:
		return true
	}
	return false
}

// members lists the members of a type declaration; the members of an enumeration
// follow its constants.
func (w *walker) members(declaration *gotreesitter.Node) []*gotreesitter.Node {
	body := w.tree.Field(declaration, fieldBody)
	if w.tree.Type(body) == typeEnumBody {
		members := w.tree.FirstOfType(body, typeEnumDeclarations)
		return append(w.constants(body), w.children(members)...)
	}
	return w.children(body)
}

func (w *walker) constants(body *gotreesitter.Node) []*gotreesitter.Node {
	var found []*gotreesitter.Node
	for _, child := range w.children(body) {
		if w.tree.Type(child) == typeEnumConstant {
			found = append(found, child)
		}
	}
	return found
}

// declare lists the units of a type and of the types nested in it. chain holds the
// types around it and setup what runs for every test of them.
func (w *walker) declare(declaration *gotreesitter.Node, chain []class, setup []*gotreesitter.Node) {
	owner := class{name: w.tree.Text(w.tree.Field(declaration, fieldName)), display: w.displayName(declaration)}
	inner := append(slices.Clone(chain), owner)
	members := w.members(declaration)
	around := slices.Clone(setup)
	for _, member := range members {
		if w.runsForEveryTest(member) {
			around = append(around, member)
		}
	}
	interfaceLike := w.tree.Type(declaration) == typeInterface || w.tree.Type(declaration) == typeAnnotationType
	for _, member := range members {
		switch w.tree.Type(member) {
		case typeMethod:
			w.method(member, inner, around)
		case typeConstructor, typeCompactCtor:
			w.units.Add(treeunits.Spec{Node: member, Qualname: w.qualified(inner, initName), Name: owner.name, Owner: owner.name, Kind: code.KindFunction})
		case typeStaticInit:
			w.units.Add(treeunits.Spec{Node: member, Qualname: w.qualified(inner, classInitName), Name: owner.name, Owner: owner.name, Kind: code.KindFunction})
		case typeField, typeConstant:
			w.constantsOf(member, inner, interfaceLike || w.tree.Type(member) == typeConstant)
		case typeEnumConstant:
			name := w.tree.Text(w.tree.Field(member, fieldName))
			w.units.Add(treeunits.Spec{Node: member, Qualname: w.qualified(inner, name), Name: name, Owner: owner.name, Kind: code.KindAssignment})
		default:
			if w.isType(member) {
				w.declare(member, inner, around)
			}
		}
	}
}

func (w *walker) qualified(chain []class, name string) string {
	parts := make([]string, 0, len(chain)+1)
	for _, c := range chain {
		parts = append(parts, c.name)
	}
	return strings.Join(append(parts, name), ".")
}

// method adds the unit of a method: a test when a JUnit 5 annotation says so, a
// function otherwise.
func (w *walker) method(method *gotreesitter.Node, chain []class, setup []*gotreesitter.Node) {
	name := w.tree.Text(w.tree.Field(method, fieldName))
	owner := chain[len(chain)-1].name
	spec := treeunits.Spec{Node: method, Qualname: w.qualified(chain, name), Name: name, Owner: owner, Kind: code.KindFunction}
	if w.hasAnnotation(method, testAnnotations...) {
		spec.Kind, spec.Also, spec.Labels = code.KindTest, setup, []string{testlabel.JUnitPattern(w.testOf(method, name, chain))}
	}
	w.units.Add(spec)
}

// constantsOf adds a unit per constant a field declaration declares: a static final
// field, or any field of an interface.
func (w *walker) constantsOf(field *gotreesitter.Node, chain []class, interfaceLike bool) {
	words := w.modifierWords(field)
	if !interfaceLike && !(slices.Contains(words, tokenStatic) && slices.Contains(words, tokenFinal)) {
		return
	}
	var declarators []*gotreesitter.Node
	for index := 0; index < field.ChildCount(); index++ {
		if field.FieldNameForChild(index, w.tree.Lang) == fieldDeclarator {
			declarators = append(declarators, field.Child(index))
		}
	}
	owner := chain[len(chain)-1].name
	for _, declarator := range declarators {
		name := w.tree.Text(w.tree.Field(declarator, fieldName))
		span := declarator
		if len(declarators) == 1 {
			span = field
		}
		w.units.Add(treeunits.Spec{Node: span, Qualname: w.qualified(chain, name), Name: name, Owner: owner, Kind: code.KindAssignment})
	}
}

// runsForEveryTest reports whether a member of a class runs for each test of it: a
// hook, an extension, an initializer or the initial value of an instance field.
func (w *walker) runsForEveryTest(member *gotreesitter.Node) bool {
	switch w.tree.Type(member) {
	case typeStaticInit, typeBlock:
		return true
	case typeMethod, typeField:
		if w.hasAnnotation(member, setupAnnotations...) {
			return true
		}
		return w.tree.Type(member) == typeField && !slices.Contains(w.modifierWords(member), tokenStatic)
	}
	return false
}

// annotations lists the annotations in the modifiers of a declaration.
func (w *walker) annotations(declaration *gotreesitter.Node) []*gotreesitter.Node {
	var found []*gotreesitter.Node
	for _, child := range w.tree.Named(w.tree.FirstOfType(declaration, typeModifiers)) {
		if kind := w.tree.Type(child); kind == typeAnnotation || kind == typeMarkerAnnotation {
			found = append(found, child)
		}
	}
	return found
}

func (w *walker) annotationName(annotation *gotreesitter.Node) string {
	name := w.tree.Text(w.tree.Field(annotation, fieldName))
	return name[strings.LastIndex(name, ".")+1:]
}

func (w *walker) hasAnnotation(declaration *gotreesitter.Node, names ...string) bool {
	return slices.ContainsFunc(w.annotations(declaration), func(a *gotreesitter.Node) bool {
		return slices.Contains(names, w.annotationName(a))
	})
}

// modifierWords lists the keyword modifiers of a declaration, without annotations.
func (w *walker) modifierWords(declaration *gotreesitter.Node) []string {
	var words []string
	for _, child := range w.tree.Children(w.tree.FirstOfType(declaration, typeModifiers)) {
		if kind := w.tree.Type(child); kind != typeAnnotation && kind != typeMarkerAnnotation {
			words = append(words, w.tree.Text(child))
		}
	}
	return words
}

// displayName is the display name of a type, empty when it has none.
func (w *walker) displayName(declaration *gotreesitter.Node) string {
	for _, annotation := range w.annotations(declaration) {
		if w.annotationName(annotation) == annotationDisplayName {
			text, _ := w.annotationText(annotation, "")
			return text
		}
	}
	return ""
}

// annotationText reads a text an annotation carries: its only argument, or the
// element with the key.
func (w *walker) annotationText(annotation *gotreesitter.Node, key string) (string, bool) {
	arguments := w.tree.Field(annotation, fieldArguments)
	for _, argument := range w.children(arguments) {
		switch w.tree.Type(argument) {
		case typeStringLiteral:
			if key == "" {
				return w.stringContent(argument), true
			}
		case typeElementPair:
			if w.tree.Text(w.tree.Field(argument, fieldKey)) == key {
				if value := w.tree.Field(argument, fieldValue); w.tree.Type(value) == typeStringLiteral {
					return w.stringContent(value), true
				}
			}
		}
	}
	return "", false
}

func (w *walker) stringContent(literal *gotreesitter.Node) string {
	var content strings.Builder
	for _, part := range w.tree.Named(literal) {
		content.WriteString(w.tree.Text(part))
	}
	return content.String()
}

// testOf reads what a runner reports about a test method from its annotations.
func (w *walker) testOf(method *gotreesitter.Node, name string, chain []class) testlabel.JUnitTest {
	test := testlabel.JUnitTest{Method: name}
	for _, c := range chain {
		test.Classes = append(test.Classes, testlabel.JUnitClass{Name: c.name, Display: c.display})
	}
	for _, annotation := range w.annotations(method) {
		switch w.annotationName(annotation) {
		case annotationDisplayName:
			test.Display, _ = w.annotationText(annotation, "")
		case annotationParameterized:
			test.Parameterized = true
			test.Template, _ = w.annotationText(annotation, elementName)
		case annotationRepeated:
			test.Repeated = true
			test.Template, _ = w.annotationText(annotation, elementName)
		}
	}
	return test
}
