// Package golang indexes the units of Go source files with the standard library's
// parser: functions, methods, and package-level variables and constants. The
// normal form of a unit is a dump of its syntax tree without positions or
// comments, so a change of formatting or of a comment is no change.
package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path"
	"reflect"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/completeness/internal/app/ports"
	"github.com/RachidChabane/musthold/completeness/internal/domain/code"
)

// Naming rules of Go source files.
const (
	sourceExtension = ".go"
	testSuffix      = "_test.go"
	blankName       = "_"
	receiverFormat  = "%s.%s"
	repeatFormat    = "%s#%d"
)

var _ ports.Indexer = Indexer{}

// Indexer implements ports.Indexer for Go.
type Indexer struct{}

// New returns the Go indexer.
func New() Indexer { return Indexer{} }

// Handles reports whether the file is a Go source file.
func (Indexer) Handles(file string) bool { return path.Ext(file) == sourceExtension }

// IsTest reports whether the file holds Go tests.
func (Indexer) IsTest(file string) bool { return strings.HasSuffix(file, testSuffix) }

// Index parses the source and lists its units in source order. A file that does
// not parse is an error.
func (i Indexer) Index(file string, source []byte) (code.File, error) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, file, source, parser.SkipObjectResolution)
	if err != nil {
		return code.File{}, fmt.Errorf("parse %s: %w", file, err)
	}
	builder := &builder{file: file, fileSet: fileSet, names: map[string]int{}}
	for _, declaration := range parsed.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			builder.function(typed)
		case *ast.GenDecl:
			builder.values(typed)
		}
	}
	return code.File{Path: file, Test: i.IsTest(file), Units: builder.units}, nil
}

type builder struct {
	file    string
	fileSet *token.FileSet
	units   []code.Unit
	names   map[string]int
}

// qualname gives a unit a name that is unique in its file: a second declaration
// of the same name, such as another `init`, gets a repeat counter.
func (b *builder) qualname(name string) string {
	b.names[name]++
	if count := b.names[name]; count > 1 {
		return fmt.Sprintf(repeatFormat, name, count)
	}
	return name
}

func (b *builder) function(declaration *ast.FuncDecl) {
	owner := receiverType(declaration)
	name := declaration.Name.Name
	qualname := name
	if owner != "" {
		qualname = fmt.Sprintf(receiverFormat, owner, name)
	}
	b.add(declaration, b.qualname(qualname), name, owner, code.KindFunction)
}

// values adds one unit per name declared by a package-level var or const.
func (b *builder) values(declaration *ast.GenDecl) {
	if declaration.Tok != token.VAR && declaration.Tok != token.CONST {
		return
	}
	for _, spec := range declaration.Specs {
		value, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}
		for _, name := range value.Names {
			if name.Name != blankName {
				b.add(value, b.qualname(name.Name), name.Name, "", code.KindAssignment)
			}
		}
	}
}

func (b *builder) add(node ast.Node, qualname, name, owner string, kind code.Kind) {
	b.units = append(b.units, code.Unit{
		File: b.file, Qualname: qualname, Name: name, Owner: owner, Kind: kind,
		Normalized: normalForm(node), Text: render(b.fileSet, node),
		StartLine: b.fileSet.Position(node.Pos()).Line, EndLine: b.fileSet.Position(node.End()).Line,
		Mentions: mentions(node),
	})
}

// positionType is the type of every position field of the syntax tree.
var positionType = reflect.TypeOf(token.NoPos)

// normalForm dumps the syntax tree of a node without its positions: two nodes with
// the same structure, names, literals and operators give the same text, whatever
// their layout.
func normalForm(node ast.Node) string {
	var dump bytes.Buffer
	keep := func(name string, value reflect.Value) bool {
		return value.Type() != positionType && ast.NotNilFilter(name, value)
	}
	if err := ast.Fprint(&dump, nil, node, keep); err != nil {
		return ""
	}
	return dump.String()
}

func render(fileSet *token.FileSet, node ast.Node) string {
	var buffer bytes.Buffer
	if err := printer.Fprint(&buffer, fileSet, node); err != nil {
		return ""
	}
	return buffer.String()
}

// receiverType names the type a method is declared on, without pointer or type
// parameters; it is empty for a function.
func receiverType(declaration *ast.FuncDecl) string {
	if declaration.Recv == nil || len(declaration.Recv.List) == 0 {
		return ""
	}
	expression := declaration.Recv.List[0].Type
	for {
		switch typed := expression.(type) {
		case *ast.StarExpr:
			expression = typed.X
		case *ast.IndexExpr:
			expression = typed.X
		case *ast.IndexListExpr:
			expression = typed.X
		case *ast.ParenExpr:
			expression = typed.X
		case *ast.Ident:
			return typed.Name
		default:
			return ""
		}
	}
}

// mentions lists the identifiers a node refers to, sorted and without repeats.
func mentions(node ast.Node) []string {
	found := map[string]bool{}
	ast.Inspect(node, func(visited ast.Node) bool {
		if identifier, isIdentifier := visited.(*ast.Ident); isIdentifier && identifier.Name != blankName {
			found[identifier.Name] = true
		}
		return true
	})
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
