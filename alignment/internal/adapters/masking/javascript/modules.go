package javascript

import (
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// maxReexportDepth bounds how far an export is followed through re-exports.
const maxReexportDepth = 8

// Roots under which a specifier that is not relative is looked up; the empty root
// is the project root.
var specifierRoots = []string{"", "src"}

// exportEntry says where an exported name is declared.
type exportEntry struct {
	// statement is the statement that declares or exports it, nil for a re-export.
	statement *gotreesitter.Node
	// local names the declaration inside the file; empty for `export default expr`.
	local string
	// value is the expression of `export default expr`.
	value *gotreesitter.Node
	// from and name designate a re-export of `name` from the module `from`.
	from, name string
}

// moduleExports is a parsed project file with what it declares and exports.
type moduleExports struct {
	*fileIndex
	locals  map[string]*gotreesitter.Node
	exports map[string]exportEntry
	// stars lists the modules the file re-exports as a whole.
	stars []string
}

func newModuleExports(file string, source []byte) (*moduleExports, error) {
	index, err := indexFile(file, source)
	if err != nil {
		return nil, err
	}
	module := &moduleExports{fileIndex: index, locals: map[string]*gotreesitter.Node{}, exports: map[string]exportEntry{}}
	exported := false
	for _, statement := range index.statementsOf(index.Root()) {
		if index.Type(statement) == typeExportStatement {
			exported = true
			module.readExport(statement)
			continue
		}
		for _, name := range index.declaredNames(statement) {
			module.locals[name] = statement
		}
	}
	if !exported {
		for name, statement := range module.locals {
			module.exports[name] = exportEntry{statement: statement, local: name}
		}
	}
	return module, nil
}

func (m *moduleExports) readExport(statement *gotreesitter.Node) {
	isDefault := slices.ContainsFunc(m.Children(statement), func(child *gotreesitter.Node) bool { return m.Text(child) == tokenDefault })
	names := m.declaredNames(statement)
	for _, name := range names {
		m.locals[name] = statement
		m.exports[name] = exportEntry{statement: statement, local: name}
	}
	switch {
	case isDefault && len(names) > 0:
		m.exports[tokenDefault] = exportEntry{statement: statement, local: names[0]}
	case isDefault:
		m.exports[tokenDefault] = exportEntry{statement: statement, value: m.Field(statement, fieldValue)}
	}
	if clause := m.FirstOfType(statement, typeExportClause); clause != nil {
		m.readClause(statement, clause)
		return
	}
	if source := m.Field(statement, fieldSource); source != nil && m.hasStar(statement) {
		m.stars = append(m.stars, m.stringContent(source))
	}
}

func (m *moduleExports) hasStar(statement *gotreesitter.Node) bool {
	return slices.ContainsFunc(m.Children(statement), func(child *gotreesitter.Node) bool { return m.Text(child) == tokenStar })
}

func (m *moduleExports) readClause(statement, clause *gotreesitter.Node) {
	source := ""
	if node := m.Field(statement, fieldSource); node != nil {
		source = m.stringContent(node)
	}
	for _, specifier := range m.Named(clause) {
		if m.Type(specifier) != typeExportSpecifier {
			continue
		}
		name := m.Text(m.Field(specifier, fieldName))
		exported := name
		if alias := m.Field(specifier, fieldAlias); alias != nil {
			exported = m.Text(alias)
		}
		if source != "" {
			m.exports[exported] = exportEntry{from: source, name: name}
			continue
		}
		m.exports[exported] = exportEntry{local: name}
	}
}

// resolver finds the project file of a module specifier.
type resolver struct{ project fs.FS }

// resolve returns the file a specifier written in the file designates, if the
// project holds it.
func (r resolver) resolve(from, specifier string) (string, bool) {
	var bases []string
	if strings.HasPrefix(specifier, ".") {
		bases = []string{path.Join(path.Dir(from), specifier)}
	} else {
		for _, root := range specifierRoots {
			bases = append(bases, path.Join(root, specifier))
		}
	}
	for _, base := range bases {
		for _, candidate := range candidates(base) {
			if info, err := fs.Stat(r.project, candidate); err == nil && !info.IsDir() {
				return candidate, true
			}
		}
	}
	return "", false
}

// candidates lists the files a module path may stand for: the path itself, the
// path with a source extension, which a TypeScript project spells as the
// extension of the compiled file, and the index of a directory.
func candidates(base string) []string {
	var found []string
	if slices.Contains(sourceExtensions, path.Ext(base)) {
		found = append(found, base)
	}
	stems := []string{base}
	if slices.Contains(javascriptExtensions, path.Ext(base)) {
		stems = append(stems, strings.TrimSuffix(base, path.Ext(base)))
	}
	for _, stem := range stems {
		for _, extension := range sourceExtensions {
			found = append(found, stem+extension)
		}
	}
	for _, extension := range sourceExtensions {
		found = append(found, path.Join(base, "index"+extension))
	}
	return found
}
