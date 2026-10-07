package golang

import "go/ast"

// referenceWalker visits the identifiers of a declaration that are references,
// not names: selector fields, struct field keys, field names, labels and the
// declared names themselves are skipped.
type referenceWalker struct {
	onIdent    func(*ast.Ident)
	onSelector func(base, field *ast.Ident)
}

// walkReferences visits the references of a node. Either callback may be nil.
func walkReferences(node ast.Node, onIdent func(*ast.Ident), onSelector func(base, field *ast.Ident)) {
	ast.Walk(referenceWalker{onIdent: onIdent, onSelector: onSelector}, node)
}

// Visit implements ast.Visitor.
func (w referenceWalker) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.Ident:
		w.ident(n)
	case *ast.SelectorExpr:
		w.selector(n)
	case *ast.Field:
		w.expr(n.Type)
	case *ast.LabeledStmt:
		w.stmt(n.Stmt)
	case *ast.BranchStmt:
	case *ast.CompositeLit:
		w.compositeLit(n)
	case *ast.FuncDecl:
		w.funcDecl(n)
	case *ast.TypeSpec:
		w.fields(n.TypeParams)
		w.expr(n.Type)
	case *ast.ValueSpec:
		w.expr(n.Type)
		w.exprs(n.Values)
	default:
		return w
	}
	return nil
}

func (w referenceWalker) ident(id *ast.Ident) {
	if w.onIdent != nil {
		w.onIdent(id)
	}
}

func (w referenceWalker) selector(n *ast.SelectorExpr) {
	w.expr(n.X)
	if base, isIdent := n.X.(*ast.Ident); isIdent && w.onSelector != nil {
		w.onSelector(base, n.Sel)
	}
}

func (w referenceWalker) funcDecl(n *ast.FuncDecl) {
	w.fields(n.Recv)
	w.expr(n.Type)
	if n.Body != nil {
		ast.Walk(w, n.Body)
	}
}

// compositeLit skips identifier keys unless the literal is a map, slice or
// array, where a key is an expression; in a struct literal it names a field.
func (w referenceWalker) compositeLit(n *ast.CompositeLit) {
	w.expr(n.Type)
	keysAreExpressions := hasExpressionKeys(n.Type)
	for _, element := range n.Elts {
		pair, isPair := element.(*ast.KeyValueExpr)
		if !isPair {
			w.expr(element)
			continue
		}
		if _, isName := pair.Key.(*ast.Ident); !isName || keysAreExpressions {
			w.expr(pair.Key)
		}
		w.expr(pair.Value)
	}
}

func hasExpressionKeys(literalType ast.Expr) bool {
	switch literalType.(type) {
	case *ast.MapType, *ast.ArrayType:
		return true
	}
	return false
}

func (w referenceWalker) expr(node ast.Expr) {
	if node != nil {
		ast.Walk(w, node)
	}
}

func (w referenceWalker) stmt(node ast.Stmt) {
	if node != nil {
		ast.Walk(w, node)
	}
}

func (w referenceWalker) exprs(nodes []ast.Expr) {
	for _, node := range nodes {
		w.expr(node)
	}
}

func (w referenceWalker) fields(list *ast.FieldList) {
	if list != nil {
		ast.Walk(w, list)
	}
}

// refersToPackageLevel reports whether an identifier of the file can name a
// package-level object: it is unresolved in the file (declared in a sibling file
// or the universe scope) or resolves to the file's own package scope.
func refersToPackageLevel(id *ast.Ident, file *sourceFile) bool {
	return id.Obj == nil || file.syntax.Scope.Objects[id.Name] == id.Obj
}
