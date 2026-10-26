package golang

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// ErrSyntax is returned for a source that does not parse.
var ErrSyntax = errors.New("go syntax error")

// parseMode keeps object resolution on: alpha-renaming relies on ast.Object.
const parseMode = parser.Mode(0)

// source is a parsed Go file.
type source struct {
	fset *token.FileSet
	file *ast.File
	text []byte
}

func parseSource(path string, content []byte) (*source, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parseMode)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSyntax, err)
	}
	return &source{fset: fset, file: file, text: content}, nil
}

func (s *source) offset(pos token.Pos) int { return s.fset.File(pos).Offset(pos) }

func (s *source) spanOf(n ast.Node) ecosystem.Span {
	return ecosystem.Span{Start: s.offset(n.Pos()), End: s.offset(n.End())}
}

func (s *source) textOf(n ast.Node) string {
	span := s.spanOf(n)
	return string(s.text[span.Start:span.End])
}

// walk visits the nodes below root depth first with their ancestors, nearest
// last; when visit returns false the children are skipped.
func walk(root ast.Node, visit func(n ast.Node, up []ast.Node) bool) {
	var stack []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if !visit(n, stack) {
			return false
		}
		stack = append(stack, n)
		return true
	})
}

// nearest returns the n-th ancestor, 1 being the parent, or nil.
func nearest(up []ast.Node, n int) ast.Node {
	if n > len(up) {
		return nil
	}
	return up[len(up)-n]
}

func replaceIn(unit string, unitStart int, span ecosystem.Span, replacement string) string {
	return unit[:span.Start-unitStart] + replacement + unit[span.End-unitStart:]
}

func isExported(name string) bool {
	return name != "" && strings.ToUpper(name[:1]) == name[:1] && strings.ToLower(name[:1]) != name[:1]
}
