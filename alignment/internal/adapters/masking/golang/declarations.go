package golang

import (
	"errors"
	"go/ast"
	"go/token"
)

const (
	initFunctionName = "init"
	blankName        = "_"
)

var errNoSuchTest = errors.New("no such top-level test function")

type declarationKind int

const (
	kindFunction declarationKind = iota
	kindType
	kindValue
	kindMethod
)

// declaration is one package-level unit of a test package: a function, a method,
// a type, or a var or const spec (a whole const group, to keep its iota
// sequence).
type declaration struct {
	file     *sourceFile
	node     ast.Decl
	kind     declarationKind
	names    []string
	receiver string
}

// catalog indexes the package-level declarations of a test package in source
// order: file name order, then position.
type catalog struct {
	ordered []*declaration
	byName  map[string]*declaration
	methods map[string][]*declaration
}

func newCatalog(files []*sourceFile) *catalog {
	c := &catalog{byName: map[string]*declaration{}, methods: map[string][]*declaration{}}
	for _, file := range files {
		for _, decl := range file.syntax.Decls {
			switch n := decl.(type) {
			case *ast.FuncDecl:
				c.addFunc(file, n)
			case *ast.GenDecl:
				c.addGen(file, n)
			}
		}
	}
	return c
}

func (c *catalog) add(decl *declaration) {
	c.ordered = append(c.ordered, decl)
	for _, name := range decl.names {
		c.byName[name] = decl
	}
	if decl.kind == kindMethod {
		c.methods[decl.receiver] = append(c.methods[decl.receiver], decl)
	}
}

func (c *catalog) addFunc(file *sourceFile, fn *ast.FuncDecl) {
	if fn.Recv != nil {
		c.add(&declaration{file: file, node: fn, kind: kindMethod, receiver: receiverTypeName(fn.Recv)})
		return
	}
	if fn.Name.Name == initFunctionName || fn.Name.Name == blankName {
		return
	}
	c.add(&declaration{file: file, node: fn, kind: kindFunction, names: []string{fn.Name.Name}})
}

func (c *catalog) addGen(file *sourceFile, gen *ast.GenDecl) {
	switch gen.Tok {
	case token.CONST:
		c.add(&declaration{file: file, node: gen, kind: kindValue, names: specNames(gen.Specs)})
	case token.TYPE, token.VAR:
		for _, spec := range gen.Specs {
			single := &ast.GenDecl{TokPos: gen.TokPos, Tok: gen.Tok, Specs: []ast.Spec{spec}}
			c.add(&declaration{file: file, node: single, kind: kindOf(gen.Tok), names: specNames([]ast.Spec{spec})})
		}
	}
}

func kindOf(tok token.Token) declarationKind {
	if tok == token.TYPE {
		return kindType
	}
	return kindValue
}

func specNames(specs []ast.Spec) []string {
	var names []string
	for _, spec := range specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			names = appendName(names, s.Name)
		case *ast.ValueSpec:
			for _, id := range s.Names {
				names = appendName(names, id)
			}
		}
	}
	return names
}

func appendName(names []string, id *ast.Ident) []string {
	if id.Name == blankName {
		return names
	}
	return append(names, id.Name)
}

// receiverTypeName is the name of the base type of a method receiver.
func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	expr := recv.List[0].Type
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.Ident:
			return e.Name
		default:
			return ""
		}
	}
}

// findTest returns the top-level function declared in the file under the name.
func (c *catalog) findTest(file *sourceFile, name string) (*declaration, error) {
	decl := c.byName[name]
	if decl == nil || decl.file != file || decl.kind != kindFunction {
		return nil, errNoSuchTest
	}
	return decl, nil
}

// reachableFrom returns the declarations reachable from the root by identifier
// name, in source order. A type brings its methods.
func (c *catalog) reachableFrom(root *declaration) []*declaration {
	included := map[*declaration]bool{root: true}
	queue := []*declaration{root}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range c.dependencies(current) {
			if !included[next] {
				included[next] = true
				queue = append(queue, next)
			}
		}
	}
	var ordered []*declaration
	for _, decl := range c.ordered {
		if included[decl] {
			ordered = append(ordered, decl)
		}
	}
	return ordered
}

func (c *catalog) dependencies(decl *declaration) []*declaration {
	var found []*declaration
	walkReferences(decl.node, func(id *ast.Ident) {
		if !refersToPackageLevel(id, decl.file) {
			return
		}
		if target := c.byName[id.Name]; target != nil {
			found = append(found, target)
		}
	}, nil)
	if decl.kind == kindType {
		for _, name := range decl.names {
			found = append(found, c.methods[name]...)
		}
	}
	return found
}
