package javascript

import (
	"io/fs"
	"slices"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/textedit"
)

// Layout of the stubs.
const (
	stubBody       = " ..."
	stubEnd        = ";"
	memberIndent   = "  "
	classOpen      = " {"
	classClose     = "}"
	emptyClass     = " {}"
	declarationSep = "\n\n"
	assignment     = " = "
	kindFieldName  = "kind"
	wordSpace      = " "
)

// signatureBuilder reads the project modules that the unit imports and writes the
// declarations it takes from them without bodies.
type signatureBuilder struct {
	project   fs.FS
	resolver  resolver
	reads     map[string]map[string]bool
	modules   map[string]*moduleExports
	stubs     []string
	published map[string]bool
	read      map[string]bool
}

// buildSignatures returns the stubs of the project declarations the imports bring
// into the unit, and the project files they were read from, sorted.
func buildSignatures(project fs.FS, testFile string, imports []importStatement, reads map[string]map[string]bool) (string, []string) {
	b := &signatureBuilder{
		project: project, resolver: resolver{project: project}, reads: reads,
		modules: map[string]*moduleExports{}, published: map[string]bool{}, read: map[string]bool{},
	}
	defer b.release()
	for _, statement := range imports {
		file, ok := b.resolver.resolve(testFile, statement.spec)
		if !ok {
			continue
		}
		for _, bound := range statement.bindings {
			b.addBinding(file, bound)
		}
	}
	return strings.Join(b.stubs, declarationSep), b.files()
}

func (b *signatureBuilder) release() {
	for _, module := range b.modules {
		if module != nil {
			module.Release()
		}
	}
}

func (b *signatureBuilder) files() []string {
	files := make([]string, 0, len(b.read))
	for file := range b.read {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}

func (b *signatureBuilder) addBinding(file string, bound binding) {
	if !bound.namespace {
		b.addNamed(file, bound.imported, 0)
		return
	}
	names := make([]string, 0, len(b.reads[bound.local]))
	for name := range b.reads[bound.local] {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		b.addNamed(file, name, 0)
	}
}

// load parses a project file once. A file that cannot be read yields nil.
func (b *signatureBuilder) load(file string) *moduleExports {
	if module, loaded := b.modules[file]; loaded {
		return module
	}
	source, err := fs.ReadFile(b.project, file)
	if err != nil {
		b.modules[file] = nil
		return nil
	}
	module, err := newModuleExports(file, source)
	if err != nil {
		b.modules[file] = nil
		return nil
	}
	b.modules[file] = module
	return module
}

// addNamed writes the stub of the name a module exports, following re-exports.
func (b *signatureBuilder) addNamed(file, name string, depth int) {
	module := b.load(file)
	if module == nil || depth > maxReexportDepth {
		return
	}
	entry, exported := module.exports[name]
	if !exported {
		b.addFromStars(module, name, depth)
		return
	}
	if entry.from != "" {
		if target, ok := b.resolver.resolve(file, entry.from); ok {
			b.addNamed(target, entry.name, depth+1)
		}
		return
	}
	key := file + "#" + name
	if b.published[key] {
		return
	}
	b.published[key] = true
	if text := module.stubOf(entry); text != "" {
		b.stubs = append(b.stubs, text)
		b.read[file] = true
	}
}

func (b *signatureBuilder) addFromStars(module *moduleExports, name string, depth int) {
	if name == tokenDefault {
		return
	}
	for _, spec := range module.stars {
		if target, ok := b.resolver.resolve(module.file, spec); ok {
			b.addNamed(target, name, depth+1)
		}
	}
}

// stubOf writes what a module exports under a name: its declaration without body.
func (m *moduleExports) stubOf(entry exportEntry) string {
	if entry.value != nil {
		return m.defaultValueStub(entry)
	}
	statement := m.locals[entry.local]
	if statement == nil {
		return ""
	}
	return m.declarationStub(statement, entry.local)
}

func (m *moduleExports) defaultValueStub(entry exportEntry) string {
	switch m.Type(entry.value) {
	case typeFunctionExpression, typeFunctionOld, typeArrowFunction:
		return m.prefixOf(entry.statement, entry.value) + m.headerTo(entry.value, m.Field(entry.value, fieldBody)) + stubEnd
	}
	return ""
}

// prefixOf is the text of an export statement before the declaration it wraps, such
// as `export default `.
func (x *fileIndex) prefixOf(statement, declaration *gotreesitter.Node) string {
	if statement == nil || statement == declaration {
		return ""
	}
	return strings.TrimSpace(string(x.Source[statement.StartByte():declaration.StartByte()])) + wordSpace
}

// headerTo is the text of the node up to the start of the end node, without
// comments; the whole node when the end is nil.
func (x *fileIndex) headerTo(node, end *gotreesitter.Node) string {
	stop := int(node.EndByte())
	if end != nil {
		stop = int(end.StartByte())
	}
	return strings.TrimSpace(textedit.Tidy(textedit.Render(x.Source, textedit.Span{Start: int(node.StartByte()), End: stop}, x.comments)))
}

// clean is the text of the node without comments.
func (x *fileIndex) clean(node *gotreesitter.Node) string { return x.headerTo(node, nil) }

func (x *fileIndex) declarationStub(statement *gotreesitter.Node, name string) string {
	declaration := x.unwrapExport(statement)
	prefix := x.prefixOf(statement, declaration)
	switch x.Type(declaration) {
	case typeFunctionDecl, typeGeneratorDecl:
		return prefix + x.headerTo(declaration, x.Field(declaration, fieldBody)) + stubEnd
	case typeFunctionSignature:
		return prefix + x.clean(declaration) + stubEnd
	case typeClassDeclaration, typeAbstractClass:
		return x.classStub(prefix, declaration)
	case typeLexical, typeVariable:
		return x.variableStub(prefix, declaration, name)
	case typeInterface, typeTypeAlias, typeEnumDeclaration:
		return prefix + x.clean(declaration)
	}
	return ""
}

func (x *fileIndex) variableStub(prefix string, declaration *gotreesitter.Node, name string) string {
	kind := x.Text(x.Field(declaration, kindFieldName))
	if kind == "" {
		kind = x.Text(declaration.Child(0))
	}
	for _, declarator := range x.Named(declaration) {
		if x.Type(declarator) != typeVariableDeclarator || x.Text(x.Field(declarator, fieldName)) != name {
			continue
		}
		stub := prefix + kind + wordSpace + name
		if annotation := x.Field(declarator, fieldType); annotation != nil {
			stub += x.clean(annotation)
		}
		if value := x.Field(declarator, fieldValue); x.isFunction(value) {
			stub += assignment + x.headerTo(value, x.Field(value, fieldBody)) + stubBody
		}
		return stub + stubEnd
	}
	return ""
}

func (x *fileIndex) isFunction(node *gotreesitter.Node) bool {
	switch x.Type(node) {
	case typeArrowFunction, typeFunctionExpression, typeFunctionOld:
		return true
	}
	return false
}

func (x *fileIndex) classStub(prefix string, class *gotreesitter.Node) string {
	body := x.Field(class, fieldBody)
	header := prefix + x.headerTo(class, body)
	var members []string
	for _, member := range x.Named(body) {
		if stub := x.memberStub(member); stub != "" {
			members = append(members, memberIndent+stub)
		}
	}
	if len(members) == 0 {
		return header + emptyClass
	}
	return header + classOpen + "\n" + strings.Join(members, "\n") + "\n" + classClose
}

func (x *fileIndex) memberStub(member *gotreesitter.Node) string {
	if x.isPrivate(member) {
		return ""
	}
	switch x.Type(member) {
	case typeMethodDefinition:
		return x.headerTo(member, x.Field(member, fieldBody)) + stubEnd
	case typePublicField, typeFieldDefinition:
		return x.fieldStub(member)
	case typeComment:
		return ""
	}
	return strings.TrimSuffix(x.clean(member), stubEnd) + stubEnd
}

func (x *fileIndex) fieldStub(field *gotreesitter.Node) string {
	value := x.Field(field, fieldValue)
	if value == nil {
		return x.clean(field) + stubEnd
	}
	declared := strings.TrimSpace(strings.TrimSuffix(x.headerTo(field, value), "="))
	if x.isFunction(value) {
		return declared + assignment + x.headerTo(value, x.Field(value, fieldBody)) + stubBody + stubEnd
	}
	return declared + stubEnd
}

// isPrivate reports whether a class member is private by its name or its
// accessibility modifier.
func (x *fileIndex) isPrivate(member *gotreesitter.Node) bool {
	if x.Type(x.Field(member, fieldName)) == typePrivateProperty {
		return true
	}
	modifier := x.FirstOfType(member, typeAccessibility)
	return modifier != nil && x.Text(modifier) == tokenPrivate
}
