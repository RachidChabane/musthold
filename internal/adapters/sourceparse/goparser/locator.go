// Package goparser locates function and method declarations of Go sources with
// the standard library's go/parser.
package goparser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/RachidChabane/spec-proof/internal/app/ports"
	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
)

const (
	packageSeparator = "."
	pointerReceiver  = "(*%s).%s"
	valueReceiver    = "%s.%s"
)

var _ ports.DefinitionLocator = (*Locator)(nil)

// Locator implements ports.DefinitionLocator for Go sources.
type Locator struct{}

// NewLocator returns a Locator.
func NewLocator() *Locator { return &Locator{} }

// Locate returns every function and method declaration of source in source
// order. A source that does not parse is an error: Go has no useful partial
// parse for a broken declaration. Functions are named `pkg.Func`, methods
// `(*T).Method` or `T.Method`; a span runs from the `func` keyword to the end
// of the body, without the doc comment.
func (*Locator) Locate(path string, source []byte) ([]ecosystem.Definition, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, source, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", path, err)
	}
	var definitions []ecosystem.Definition
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		start, end := fileSet.Position(function.Pos()), fileSet.Position(function.End())
		definitions = append(definitions, ecosystem.Definition{
			Qualname:  qualname(file.Name.Name, function),
			Kind:      kind(function),
			Span:      ecosystem.Span{Start: start.Offset, End: end.Offset},
			StartLine: start.Line,
			EndLine:   end.Line,
		})
	}
	return definitions, nil
}

func kind(function *ast.FuncDecl) ecosystem.DefinitionKind {
	if function.Recv != nil {
		return ecosystem.KindMethod
	}
	return ecosystem.KindFunction
}

func qualname(packageName string, function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return packageName + packageSeparator + function.Name.Name
	}
	receiver, pointer := receiverType(function.Recv.List[0].Type)
	if pointer {
		return fmt.Sprintf(pointerReceiver, receiver, function.Name.Name)
	}
	return fmt.Sprintf(valueReceiver, receiver, function.Name.Name)
}

// receiverType names the receiver's type, without type arguments, and tells
// whether it is a pointer.
func receiverType(expression ast.Expr) (string, bool) {
	switch typed := expression.(type) {
	case *ast.StarExpr:
		name, _ := receiverType(typed.X)
		return name, true
	case *ast.IndexExpr:
		return receiverType(typed.X)
	case *ast.IndexListExpr:
		return receiverType(typed.X)
	case *ast.ParenExpr:
		return receiverType(typed.X)
	case *ast.Ident:
		return typed.Name, false
	default:
		return "", false
	}
}
