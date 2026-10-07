package python

import (
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// Layout of the stubs.
const (
	stubIndent     = "    "
	stubBody       = "..."
	annotationJoin = ": "
	relativeDot    = '.'
	pythonSuffix   = ".py"
	packageInit    = "__init__.py"
	sourceRoot     = "src"
	pathSeparator  = "/"
)

// Roots under which an absolute import is looked up; the empty root is the
// project root.
var importRoots = []string{"", sourceRoot}

// signatureBuilder reads the project modules that the unit imports and writes
// the signatures of the names it takes from them.
type signatureBuilder struct {
	project   fs.FS
	testDir   string
	reads     map[string]map[string]bool
	indexes   map[string]*fileIndex
	stubs     []string
	published map[string]bool
}

// buildSignatures returns the stubs of the project definitions the imports
// bring into the unit, and the project files they were read from, sorted.
func buildSignatures(project fs.FS, testFile string, imports []importStatement, reads map[string]map[string]bool) (string, []string) {
	b := &signatureBuilder{
		project:   project,
		testDir:   path.Dir(testFile),
		reads:     reads,
		indexes:   map[string]*fileIndex{},
		published: map[string]bool{},
	}
	defer b.release()
	for _, statement := range imports {
		for _, name := range statement.names {
			b.addImported(statement, name)
		}
	}
	return strings.Join(b.stubs, unitSeparator), b.files()
}

func (b *signatureBuilder) release() {
	for _, index := range b.indexes {
		index.release()
	}
}

func (b *signatureBuilder) files() []string {
	files := make([]string, 0, len(b.indexes))
	for file := range b.indexes {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}

func (b *signatureBuilder) addImported(statement importStatement, name importedName) {
	if !name.fromImport() {
		b.addAttributes(name.module, name.accessKey())
		return
	}
	if file, ok := b.resolve(statement.fromModule); ok {
		b.addNamed(file, name.name)
	}
	b.addAttributes(submodule(statement.fromModule, name.name), name.accessKey())
}

// submodule spells the module `from module import name` may bring in.
func submodule(module, name string) string {
	if strings.HasSuffix(module, string(relativeDot)) {
		return module + name
	}
	return module + classnameSeparator + name
}

// addAttributes writes the stubs of the names the unit reads from a module
// imported as a whole.
func (b *signatureBuilder) addAttributes(module, accessKey string) {
	file, ok := b.resolve(module)
	if !ok {
		return
	}
	names := make([]string, 0, len(b.reads[accessKey]))
	for name := range b.reads[accessKey] {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		b.addNamed(file, name)
	}
}

func (b *signatureBuilder) addNamed(file, name string) {
	index := b.load(file)
	if index == nil {
		return
	}
	found, ok := index.declarations[name]
	key := file + "#" + name
	if !ok || found.definition == nil || b.published[key] {
		return
	}
	b.published[key] = true
	b.stubs = append(b.stubs, index.stub(found.definition))
}

// load parses a project file once. A file that cannot be read yields nil.
func (b *signatureBuilder) load(file string) *fileIndex {
	if index, ok := b.indexes[file]; ok {
		return index
	}
	source, err := fs.ReadFile(b.project, file)
	if err != nil {
		return nil
	}
	index, err := indexFile(source)
	if err != nil {
		return nil
	}
	b.indexes[file] = index
	return index
}

// resolve finds the project file of a module, relative to the test for a
// relative import and under the project roots otherwise.
func (b *signatureBuilder) resolve(module string) (string, bool) {
	for _, candidate := range b.candidates(module) {
		if info, err := fs.Stat(b.project, candidate); err == nil && !info.IsDir() {
			if b.load(candidate) != nil {
				return candidate, true
			}
		}
	}
	return "", false
}

func (b *signatureBuilder) candidates(module string) []string {
	dots := len(module) - len(strings.TrimLeft(module, string(relativeDot)))
	rest := module[dots:]
	if dots == 0 {
		var found []string
		for _, root := range importRoots {
			found = append(found, modulePaths(root, rest)...)
		}
		return found
	}
	base := b.testDir
	for level := 1; level < dots; level++ {
		base = path.Dir(base)
	}
	return modulePaths(base, rest)
}

func modulePaths(root, module string) []string {
	if module == "" {
		return []string{path.Join(root, packageInit)}
	}
	file := path.Join(root, strings.ReplaceAll(module, classnameSeparator, pathSeparator))
	return []string{file + pythonSuffix, path.Join(file, packageInit)}
}

// stub writes the signature of a function or class definition without decorators
// or bodies.
func (x *fileIndex) stub(definition *gotreesitter.Node) string {
	if x.typeOf(definition) != typeClass {
		return x.signatureWithBody(definition, "")
	}
	members := x.classMembers(definition)
	if len(members) == 0 {
		members = []string{stubIndent + stubBody}
	}
	return strings.Join(append([]string{x.header(definition)}, members...), lineSeparator)
}

func (x *fileIndex) signatureWithBody(definition *gotreesitter.Node, indent string) string {
	return indent + x.header(definition) + lineSeparator + indent + stubIndent + stubBody
}

func (x *fileIndex) classMembers(class *gotreesitter.Node) []string {
	body := x.child(class, fieldBody)
	if body == nil {
		return nil
	}
	var members []string
	for _, statement := range x.statementsOf(body) {
		if member, ok := x.member(statement); ok {
			members = append(members, member)
		}
	}
	return members
}

// member is the stub of a method or the line of an annotated attribute.
func (x *fileIndex) member(statement *gotreesitter.Node) (string, bool) {
	if definition := x.definitionOf(statement); definition != nil {
		if x.typeOf(definition) != typeFunction {
			return "", false
		}
		return x.signatureWithBody(definition, stubIndent), true
	}
	assignment := x.bare(statement)
	if x.typeOf(assignment) != typeAssignment {
		return "", false
	}
	annotation := x.child(assignment, fieldType)
	if annotation == nil {
		return "", false
	}
	return stubIndent + x.text(x.child(assignment, fieldLeft)) + annotationJoin + x.text(annotation), true
}

// header is the text of a definition from its first token to the colon that
// opens its body, without comments.
func (x *fileIndex) header(definition *gotreesitter.Node) string {
	end := int(definition.EndByte())
	for _, child := range x.childrenOf(definition) {
		if x.typeOf(child) == typeColon {
			end = int(child.EndByte())
			break
		}
	}
	within := span{start: int(definition.StartByte()), end: end}
	return tidy(render(x.source, within, x.scan(definition).removals))
}
