package golang

import (
	"go/ast"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/internal/domain/mutation/footprint"
)

// callGraph builds the graph contribution of the file: one node per function or
// method, and every package-level const or var name as an assignment.
func (s *source) callGraph(file string) footprint.FileGraph {
	graph := footprint.FileGraph{File: file}
	pkg := s.file.Name.Name
	for _, decl := range s.file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Body != nil {
				graph.Nodes = append(graph.Nodes, s.nodeOf(s.functionUnit(pkg, d)))
			}
		case *ast.GenDecl:
			graph.Assignments = append(graph.Assignments, packageLevelNames(pkg, d)...)
		}
	}
	return graph
}

func packageLevelNames(pkg string, d *ast.GenDecl) []footprint.Assignment {
	var names []footprint.Assignment
	for _, spec := range d.Specs {
		if value, ok := spec.(*ast.ValueSpec); ok {
			for _, name := range value.Names {
				names = append(names, footprint.Assignment{Name: name.Name, Qualname: pkg + qualnameSeparator + name.Name})
			}
		}
	}
	return names
}

func (s *source) nodeOf(u unit) footprint.Node {
	node := footprint.Node{Qualname: u.qualname, Name: u.name, Owner: u.owner}
	receiver := receiverName(u.function)
	calls := map[footprint.Reference]bool{}
	reads := map[string]bool{}
	walk(u.node, func(n ast.Node, up []ast.Node) bool {
		switch e := n.(type) {
		case *ast.CallExpr:
			if ref, ok := callReference(e, receiver); ok {
				calls[ref] = true
			}
		case *ast.Ident:
			if selector, isSelector := nearest(up, 1).(*ast.SelectorExpr); !isSelector || selector.Sel != e {
				reads[e.Name] = true
				calls[footprint.Reference{Name: e.Name, Kind: footprint.ReferencePlain}] = true
			}
		}
		return true
	})
	for ref := range calls {
		node.Calls = append(node.Calls, ref)
	}
	slices.SortFunc(node.Calls, func(a, b footprint.Reference) int {
		return strings.Compare(string(a.Kind)+a.Name, string(b.Kind)+b.Name)
	})
	for name := range reads {
		node.Reads = append(node.Reads, name)
	}
	slices.Sort(node.Reads)
	return node
}

func receiverName(d *ast.FuncDecl) string {
	if d == nil || d.Recv == nil || len(d.Recv.List) == 0 || len(d.Recv.List[0].Names) == 0 {
		return ""
	}
	return d.Recv.List[0].Names[0].Name
}

// callReference classifies the callee of a call written as `f()` or `x.f()`;
// `f()` is already covered by the identifier references of the body.
func callReference(call *ast.CallExpr, receiver string) (footprint.Reference, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return footprint.Reference{}, false
	}
	kind := footprint.ReferenceMember
	if base, isIdent := selector.X.(*ast.Ident); isIdent && receiver != "" && base.Name == receiver {
		kind = footprint.ReferenceOwn
	}
	return footprint.Reference{Name: selector.Sel.Name, Kind: kind}, true
}
