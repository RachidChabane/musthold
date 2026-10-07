package golang

import (
	"go/ast"
	"go/token"
	"io/fs"
	"sort"
	"strings"
)

// libraryReference names a declaration of the code under test the unit uses.
type libraryReference struct {
	dir  string
	name string
}

// signature is the rendered public signature of one declaration and the file it
// was read from.
type signature struct {
	text string
	file string
}

// signatureBuilder renders the signatures of the code under test that a unit
// reaches: through selectors of in-module packages and, in a test package that
// is not external, through identifiers of the package under test.
type signatureBuilder struct {
	fset      *token.FileSet
	project   fs.FS
	pkg       *testPackage
	functions *catalog
	module    moduleInfo
	code      map[string][]*sourceFile
}

func newSignatureBuilder(fset *token.FileSet, project fs.FS, pkg *testPackage, functions *catalog) *signatureBuilder {
	return &signatureBuilder{
		fset:      fset,
		project:   project,
		pkg:       pkg,
		functions: functions,
		module:    findModule(project, pkg.dir),
		code:      map[string][]*sourceFile{},
	}
}

// build renders the signatures and lists the code files they were read from.
func (b *signatureBuilder) build(decls []*declaration) (string, []string, error) {
	unique := map[string]signature{}
	for _, ref := range b.references(decls) {
		files, err := b.codeFiles(ref.dir)
		if err != nil {
			return "", nil, err
		}
		found, err := b.signaturesOf(files, ref.name)
		if err != nil {
			return "", nil, err
		}
		for _, item := range found {
			unique[item.text] = item
		}
	}
	return joinSignatures(unique), codeFilePaths(unique), nil
}

func joinSignatures(unique map[string]signature) string {
	texts := make([]string, 0, len(unique))
	for text := range unique {
		texts = append(texts, text)
	}
	sort.Strings(texts)
	if len(texts) == 0 {
		return ""
	}
	return strings.Join(texts, declarationBreak) + "\n"
}

func codeFilePaths(unique map[string]signature) []string {
	files := map[string]bool{}
	for _, item := range unique {
		files[item.file] = true
	}
	return sortedKeys(files)
}

func (b *signatureBuilder) codeFiles(dir string) ([]*sourceFile, error) {
	if files, loaded := b.code[dir]; loaded {
		return files, nil
	}
	files, err := parseDirectory(b.fset, b.project, dir, isCodeFile)
	if err != nil {
		return nil, err
	}
	b.code[dir] = files
	return files, nil
}

// references lists the declarations of the code under test the declarations use.
func (b *signatureBuilder) references(decls []*declaration) []libraryReference {
	var found []libraryReference
	for _, decl := range decls {
		found = append(found, b.selectorReferences(decl)...)
		if !b.pkg.isExternal() {
			found = append(found, b.identifierReferences(decl)...)
		}
	}
	return found
}

func (b *signatureBuilder) selectorReferences(decl *declaration) []libraryReference {
	var found []libraryReference
	walkReferences(decl.node, nil, func(base, field *ast.Ident) {
		entry, imported := decl.file.imports[base.Name]
		if !imported || base.Obj != nil {
			return
		}
		if dir, inModule := b.module.directoryOfImport(entry.path); inModule {
			found = append(found, libraryReference{dir: dir, name: field.Name})
		}
	})
	return found
}

func (b *signatureBuilder) identifierReferences(decl *declaration) []libraryReference {
	var found []libraryReference
	walkReferences(decl.node, func(id *ast.Ident) {
		if refersToPackageLevel(id, decl.file) && b.functions.byName[id.Name] == nil {
			found = append(found, libraryReference{dir: b.pkg.dir, name: id.Name})
		}
	}, nil)
	return found
}

// signaturesOf renders the declaration of a name in the files of a package,
// with the exported methods of a type.
func (b *signatureBuilder) signaturesOf(files []*sourceFile, name string) ([]signature, error) {
	var found []signature
	for _, file := range files {
		for _, decl := range file.syntax.Decls {
			for _, shown := range signatureNodes(file, decl, files, name) {
				text, err := printNode(b.fset, shown.node)
				if err != nil {
					return nil, err
				}
				found = append(found, signature{text: text, file: shown.file})
			}
		}
	}
	return found, nil
}

type shownNode struct {
	node ast.Node
	file string
}

// signatureNodes returns the nodes to print for the declaration if it declares
// the name: a bodiless function, a type or value spec, and for a type its
// exported methods.
func signatureNodes(file *sourceFile, decl ast.Decl, files []*sourceFile, name string) []shownNode {
	switch n := decl.(type) {
	case *ast.FuncDecl:
		if n.Recv == nil && n.Name.Name == name {
			return []shownNode{{node: withoutBody(n), file: file.path}}
		}
	case *ast.GenDecl:
		return genSignatureNodes(file, n, files, name)
	}
	return nil
}

func genSignatureNodes(file *sourceFile, gen *ast.GenDecl, files []*sourceFile, name string) []shownNode {
	var shown []shownNode
	for _, spec := range gen.Specs {
		if !declares(spec, name) {
			continue
		}
		shown = append(shown, shownNode{node: &ast.GenDecl{Tok: gen.Tok, Specs: []ast.Spec{spec}}, file: file.path})
		if _, isType := spec.(*ast.TypeSpec); isType {
			shown = append(shown, exportedMethods(files, name)...)
		}
	}
	return shown
}

func declares(spec ast.Spec, name string) bool {
	return containsName(specNames([]ast.Spec{spec}), name)
}

func containsName(names []string, name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}

func exportedMethods(files []*sourceFile, typeName string) []shownNode {
	var shown []shownNode
	for _, file := range files {
		for _, decl := range file.syntax.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if isFunc && fn.Recv != nil && fn.Name.IsExported() && receiverTypeName(fn.Recv) == typeName {
				shown = append(shown, shownNode{node: withoutBody(fn), file: file.path})
			}
		}
	}
	return shown
}

func withoutBody(fn *ast.FuncDecl) *ast.FuncDecl {
	return &ast.FuncDecl{Recv: fn.Recv, Name: fn.Name, Type: fn.Type}
}
