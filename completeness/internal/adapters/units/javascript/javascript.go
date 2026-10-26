// Package javascript indexes the units of JavaScript and TypeScript source files
// with the pure-Go tree-sitter runtime: functions, methods, constructors, and
// module-level constants. A test of Jest or Vitest, which is a callback and not a
// function with a name, is a unit of kind code.KindTest that lists the labels the
// runner may report for it. The normal form of a unit is the stream of its syntax
// tree's leaves without comments, so that a change of formatting or of a comment is
// no change.
package javascript

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/RachidChabane/musthold/completeness/internal/app/ports"
	"github.com/RachidChabane/musthold/completeness/internal/domain/code"
	"github.com/RachidChabane/musthold/completeness/internal/platform/treeunits"
)

// Naming rules of JavaScript and TypeScript source files.
var (
	javascriptExtensions = []string{".js", ".jsx", ".mjs", ".cjs"}
	typescriptExtensions = []string{".ts", ".mts", ".cts"}
	tsxExtensions        = []string{".tsx"}
	// testDirectories are the directory names whose files hold tests.
	testDirectories = []string{"test", "tests", "__tests__", "__mocks__"}
	// testInfixes mark the file name of a test: `cart.test.ts`, `cart.spec.js`.
	testInfixes = []string{".test.", ".spec."}
)

// Node types and fields of the tree-sitter JavaScript and TypeScript grammars.
const (
	typeAbstractClass      = "abstract_class_declaration"
	typeArrowFunction      = "arrow_function"
	typeCallExpression     = "call_expression"
	typeClassDeclaration   = "class_declaration"
	typeComment            = "comment"
	typeEnumDeclaration    = "enum_declaration"
	typeError              = "ERROR"
	typeExportStatement    = "export_statement"
	typeExpressionStatment = "expression_statement"
	typeFieldDefinition    = "field_definition"
	typeFunctionDecl       = "function_declaration"
	typeFunctionExpression = "function_expression"
	typeFunctionOld        = "function"
	typeGeneratorDecl      = "generator_function_declaration"
	typeIdentifier         = "identifier"
	typeLexical            = "lexical_declaration"
	typeMemberExpression   = "member_expression"
	typeMethodDefinition   = "method_definition"
	typePropertyIdentifier = "property_identifier"
	typePublicField        = "public_field_definition"
	typeShorthandPattern   = "shorthand_property_identifier_pattern"
	typeShorthandProperty  = "shorthand_property_identifier"
	typeStatementBlock     = "statement_block"
	typeString             = "string"
	typeTemplateString     = "template_string"
	typeTypeIdentifier     = "type_identifier"
	typeVariable           = "variable_declaration"
	typeVariableDeclarator = "variable_declarator"
	fieldArguments         = "arguments"
	fieldBody              = "body"
	fieldDeclaration       = "declaration"
	fieldFunction          = "function"
	fieldName              = "name"
	fieldObject            = "object"
	fieldProperty          = "property"
	fieldValue             = "value"
	tokenStatic            = "static"
	tokenConstructor       = "constructor"
	tokenDefault           = "default"
	qualifierFormat        = "%s.%s"
	titleSeparator         = " > "
	patternSeparator       = `\s+`
	wholeTitle             = `.+`
	dynamicTitle           = "<title>"
)

// rules tell the comments and the identifiers of the grammars.
var rules = treeunits.Rules{
	Comments:    []string{typeComment},
	Identifiers: []string{typeIdentifier, typePropertyIdentifier, typeTypeIdentifier, typeShorthandProperty, typeShorthandPattern},
}

// Vocabulary of the runners.
var (
	suiteNames     = []string{"describe", "suite", "context"}
	testNames      = []string{"it", "test", "specify"}
	hookNames      = []string{"beforeEach", "afterEach", "beforeAll", "afterAll", "before", "after"}
	setupNames     = []string{"jest", "vi"}
	tableModifiers = []string{"each", "for"}

	tablePlaceholders    = regexp.MustCompile(`%[a-zA-Z#%]|\$\{[^}]*\}|\$[A-Za-z_#][\w.#]*`)
	templatePlaceholders = regexp.MustCompile(`\$\{[^}]*\}`)
)

var _ ports.Indexer = Indexer{}

// Indexer implements ports.Indexer for JavaScript and TypeScript.
type Indexer struct{}

// New returns the JavaScript and TypeScript indexer.
func New() Indexer { return Indexer{} }

// Handles reports whether the file is a JavaScript or TypeScript source file.
func (Indexer) Handles(file string) bool {
	_, handled := grammarOf(file)
	return handled
}

func grammarOf(file string) (*gotreesitter.Language, bool) {
	extension := path.Ext(file)
	switch {
	case slices.Contains(javascriptExtensions, extension):
		return grammars.JavascriptLanguage(), true
	case slices.Contains(typescriptExtensions, extension):
		return grammars.TypescriptLanguage(), true
	case slices.Contains(tsxExtensions, extension):
		return grammars.TsxLanguage(), true
	}
	return nil, false
}

// IsTest reports whether the file holds tests: a `*.test.*` or `*.spec.*` file, or a
// file below a `test`, `tests`, `__tests__` or `__mocks__` directory.
func (Indexer) IsTest(file string) bool {
	base := path.Base(file)
	if slices.ContainsFunc(testInfixes, func(infix string) bool { return strings.Contains(base, infix) }) {
		return true
	}
	return slices.ContainsFunc(strings.Split(path.Dir(file), "/"), func(segment string) bool {
		return slices.Contains(testDirectories, segment)
	})
}

// Index parses the source and lists its units in source order. The grammar recovers
// from syntax errors, so a broken file still yields the units it could read.
func (i Indexer) Index(file string, source []byte) (code.File, error) {
	lang, handled := grammarOf(file)
	if !handled {
		return code.File{}, fmt.Errorf("index %s: no grammar serves the file", file)
	}
	tree, err := treeunits.Parse(lang, source)
	if err != nil {
		return code.File{}, fmt.Errorf("parse %s: %w", file, err)
	}
	defer tree.Release()
	walk := &walker{tree: tree, units: treeunits.NewCollector(tree, file, rules)}
	statements := walk.statementsOf(tree.Root())
	for _, statement := range statements {
		walk.declaration(statement)
	}
	if i.IsTest(file) {
		walk.tests(statements, nil, nil)
	}
	return code.File{Path: file, Test: i.IsTest(file), Units: walk.units.Units()}, nil
}

type walker struct {
	tree  *treeunits.Tree
	units *treeunits.Collector
}

func (w *walker) statementsOf(container *gotreesitter.Node) []*gotreesitter.Node {
	var statements []*gotreesitter.Node
	for _, child := range w.tree.Named(container) {
		switch w.tree.Type(child) {
		case typeComment:
		case typeError:
			statements = append(statements, w.statementsOf(child)...)
		default:
			statements = append(statements, child)
		}
	}
	return statements
}

// declaration lists the units a top-level statement declares.
func (w *walker) declaration(statement *gotreesitter.Node) {
	inner := statement
	if w.tree.Type(statement) == typeExportStatement {
		if declared := w.tree.Field(statement, fieldDeclaration); declared != nil {
			inner = declared
		}
	}
	switch w.tree.Type(inner) {
	case typeFunctionDecl, typeGeneratorDecl:
		w.named(statement, inner, code.KindFunction)
	case typeClassDeclaration, typeAbstractClass:
		w.class(inner)
	case typeEnumDeclaration:
		w.named(statement, inner, code.KindAssignment)
	case typeLexical, typeVariable:
		w.variables(statement, inner)
	case typeExportStatement:
		w.defaultExport(inner)
	}
}

// named adds the unit of a declaration with a name; span is the node that carries
// its text.
func (w *walker) named(span, declaration *gotreesitter.Node, kind code.Kind) {
	name := w.tree.Text(w.tree.Field(declaration, fieldName))
	if name == "" {
		return
	}
	w.units.Add(treeunits.Spec{Node: span, Qualname: name, Name: name, Kind: kind})
}

// defaultExport adds the unit of `export default function () {}`, which has no name.
func (w *walker) defaultExport(statement *gotreesitter.Node) {
	value := w.tree.Field(statement, fieldValue)
	if w.isFunction(value) {
		w.units.Add(treeunits.Spec{Node: statement, Qualname: tokenDefault, Name: tokenDefault, Kind: code.KindFunction})
	}
}

func (w *walker) isFunction(node *gotreesitter.Node) bool {
	switch w.tree.Type(node) {
	case typeArrowFunction, typeFunctionExpression, typeFunctionOld:
		return true
	}
	return false
}

// variables adds one unit per name a declaration binds: a function when the value is
// one, an assignment otherwise. A declaration of several names gives each its own
// declarator as the text of the unit.
func (w *walker) variables(statement, declaration *gotreesitter.Node) {
	var declarators []*gotreesitter.Node
	for _, child := range w.tree.Named(declaration) {
		if w.tree.Type(child) == typeVariableDeclarator {
			declarators = append(declarators, child)
		}
	}
	for _, declarator := range declarators {
		name := w.tree.Field(declarator, fieldName)
		if w.tree.Type(name) != typeIdentifier {
			continue
		}
		kind := code.KindAssignment
		if w.isFunction(w.tree.Field(declarator, fieldValue)) {
			kind = code.KindFunction
		}
		span := declarator
		if len(declarators) == 1 {
			span = statement
		}
		w.units.Add(treeunits.Spec{Node: span, Qualname: w.tree.Text(name), Name: w.tree.Text(name), Kind: kind})
	}
}

// class adds the units of the members of a class: its methods, the functions its
// fields hold and its static constants. A constructor answers to the name of the
// class, which is how code instantiates it.
func (w *walker) class(class *gotreesitter.Node) {
	owner := w.tree.Text(w.tree.Field(class, fieldName))
	body := w.tree.Field(class, fieldBody)
	for _, member := range w.tree.Named(body) {
		switch w.tree.Type(member) {
		case typeMethodDefinition:
			w.method(member, owner)
		case typePublicField, typeFieldDefinition:
			w.field(member, owner)
		}
	}
}

func (w *walker) method(method *gotreesitter.Node, owner string) {
	name := w.tree.Text(w.tree.Field(method, fieldName))
	if name == "" {
		return
	}
	unitName := name
	if name == tokenConstructor {
		unitName = owner
	}
	w.units.Add(treeunits.Spec{Node: method, Qualname: fmt.Sprintf(qualifierFormat, owner, name), Name: unitName, Owner: owner, Kind: code.KindFunction})
}

func (w *walker) field(field *gotreesitter.Node, owner string) {
	nameNode := w.tree.Field(field, fieldName)
	name := w.tree.Text(nameNode)
	value := w.tree.Field(field, fieldValue)
	switch {
	case name == "" || value == nil:
	case w.isFunction(value):
		w.units.Add(treeunits.Spec{Node: field, Qualname: fmt.Sprintf(qualifierFormat, owner, name), Name: name, Owner: owner, Kind: code.KindFunction})
	case w.isStatic(field):
		w.units.Add(treeunits.Spec{Node: field, Qualname: fmt.Sprintf(qualifierFormat, owner, name), Name: name, Owner: owner, Kind: code.KindAssignment})
	}
}

func (w *walker) isStatic(field *gotreesitter.Node) bool {
	for _, child := range w.tree.Children(field) {
		if w.tree.Text(child) == tokenStatic {
			return true
		}
	}
	return false
}
