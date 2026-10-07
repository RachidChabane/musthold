package golang

import (
	"fmt"
	"go/ast"
)

const (
	neutralTestName      = "TestCase"
	helperNamePrefix     = "helper"
	helperTypeNamePrefix = "Helper"
)

// renameDeclarations gives the root and the helpers neutral names. Helper
// functions, vars and consts become helperN, helper types HelperN, numbered in
// source order. Methods, fields, selector fields and labels keep their names.
func renameDeclarations(root *declaration, decls []*declaration) {
	renames := neutralNames(root, decls)
	for _, decl := range decls {
		renameDeclared(decl.node, renames)
		walkReferences(decl.node, func(id *ast.Ident) {
			if replacement, renamed := renames[id.Name]; renamed && refersToPackageLevel(id, decl.file) {
				id.Name = replacement
			}
		}, nil)
	}
}

func neutralNames(root *declaration, decls []*declaration) map[string]string {
	renames := map[string]string{}
	counter := 0
	for _, decl := range decls {
		for _, name := range decl.names {
			if decl == root {
				renames[name] = neutralTestName
				continue
			}
			counter++
			renames[name] = helperName(decl.kind, counter)
		}
	}
	return renames
}

func helperName(kind declarationKind, number int) string {
	if kind == kindType {
		return fmt.Sprintf("%s%d", helperTypeNamePrefix, number)
	}
	return fmt.Sprintf("%s%d", helperNamePrefix, number)
}

// renameDeclared renames the names a declaration introduces.
func renameDeclared(node ast.Decl, renames map[string]string) {
	switch n := node.(type) {
	case *ast.FuncDecl:
		if n.Recv == nil {
			renameIdent(n.Name, renames)
		}
	case *ast.GenDecl:
		for _, spec := range n.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				renameIdent(s.Name, renames)
			case *ast.ValueSpec:
				for _, id := range s.Names {
					renameIdent(id, renames)
				}
			}
		}
	}
}

func renameIdent(id *ast.Ident, renames map[string]string) {
	if replacement, renamed := renames[id.Name]; renamed {
		id.Name = replacement
	}
}
