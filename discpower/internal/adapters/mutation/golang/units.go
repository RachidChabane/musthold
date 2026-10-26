package golang

import (
	"go/ast"
	"go/token"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
)

const (
	qualnameSeparator = "."
	pointerPrefix     = "(*"
	pointerSuffix     = ")"
	iotaName          = "iota"
)

// unit is what an edit replaces: a function or method declaration, or a
// package-level const or var spec of a single name.
type unit struct {
	qualname string
	name     string
	owner    string
	target   mutation.TargetKind
	node     ast.Node
	span     ecosystem.Span
	text     string
	// function is set for a declaration.
	function *ast.FuncDecl
}

// units lists the units of the file in source order. Qualnames follow the
// grammar `pkg.Func`, `(*T).Method` and `T.Method`.
func (s *source) units() []unit {
	var found []unit
	pkg := s.file.Name.Name
	for _, decl := range s.file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Body != nil {
				found = append(found, s.functionUnit(pkg, d))
			}
		case *ast.GenDecl:
			if d.Tok == token.CONST || d.Tok == token.VAR {
				found = append(found, s.specUnits(pkg, d)...)
			}
		}
	}
	return found
}

func (s *source) functionUnit(pkg string, d *ast.FuncDecl) unit {
	u := unit{name: d.Name.Name, target: mutation.TargetDefinition, node: d, function: d}
	if d.Recv == nil || len(d.Recv.List) == 0 {
		u.qualname = pkg + qualnameSeparator + d.Name.Name
	} else {
		receiver, pointer := receiverType(d.Recv.List[0].Type)
		u.owner = receiver
		if pointer {
			u.qualname = pointerPrefix + receiver + pointerSuffix + qualnameSeparator + d.Name.Name
		} else {
			u.qualname = receiver + qualnameSeparator + d.Name.Name
		}
	}
	u.span = s.spanOf(d)
	u.text = s.textOf(d)
	return u
}

// receiverType names the base type of a receiver and tells whether it is a
// pointer; type parameters are dropped.
func receiverType(expr ast.Expr) (string, bool) {
	switch t := expr.(type) {
	case *ast.StarExpr:
		name, _ := receiverType(t.X)
		return name, true
	case *ast.IndexExpr:
		return receiverType(t.X)
	case *ast.IndexListExpr:
		return receiverType(t.X)
	case *ast.ParenExpr:
		return receiverType(t.X)
	case *ast.Ident:
		return t.Name, false
	}
	return "", false
}

// specUnits returns the specs that bind exactly one name to one value and do not
// depend on iota or on an implicit repetition.
func (s *source) specUnits(pkg string, d *ast.GenDecl) []unit {
	var found []unit
	for _, spec := range d.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok || len(value.Names) != 1 || len(value.Values) != 1 || usesIota(value) {
			continue
		}
		name := value.Names[0].Name
		if name == "_" {
			continue
		}
		found = append(found, unit{
			qualname: pkg + qualnameSeparator + name,
			name:     name,
			target:   mutation.TargetModuleAssignment,
			node:     value,
			span:     s.spanOf(value),
			text:     s.textOf(value),
		})
	}
	return found
}

func usesIota(spec *ast.ValueSpec) bool {
	found := false
	ast.Inspect(spec, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == iotaName {
			found = true
		}
		return !found
	})
	return found
}
