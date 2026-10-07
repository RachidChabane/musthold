package kotlin

import (
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/textedit"
)

// Layout of the stubs.
const (
	stubIndent     = "    "
	stubOpen       = " {"
	stubClose      = "}"
	declarationGap = "\n\n"
	sourceSuffix   = ".kt"
	segmentJoin    = "/"
	wordJoin       = " "
	constantJoin   = ", "
)

// sourceRoots are the directories a package is looked up under; the empty root is
// the project root.
var sourceRoots = []string{"", "src", "src/main/kotlin", "src/test/kotlin", "src/main/java", "src/test/java"}

// signatureBuilder reads the project declarations the unit refers to and writes
// them without bodies.
type signatureBuilder struct {
	project   fs.FS
	roots     []string
	files     map[string]*fileIndex
	stubs     []string
	published map[string]bool
	read      map[string]bool
}

// signatureQuery is what the unit refers to.
type signatureQuery struct {
	testFile    string
	packageName string
	imports     []importDeclaration
	// used is every name of the unit; declared those it declares itself.
	used, declared map[string]bool
}

// buildSignatures returns the stubs of the project declarations the unit imports
// or names from its own package, and the project files they were read from, sorted.
func buildSignatures(project fs.FS, query signatureQuery) (string, []string) {
	b := &signatureBuilder{
		project: project, roots: rootsOf(query.testFile, query.packageName),
		files: map[string]*fileIndex{}, published: map[string]bool{}, read: map[string]bool{},
	}
	defer b.release()
	bound := map[string]bool{}
	packages := []string{query.packageName}
	for _, declaration := range query.imports {
		if declaration.wildcard {
			packages = append(packages, declaration.path)
			continue
		}
		bound[declaration.simple()] = true
		b.addQualified(declaration.path)
	}
	for _, name := range sortedNames(query.used) {
		if bound[name] || query.declared[name] {
			continue
		}
		for _, owner := range packages {
			if b.addQualified(joinPath(owner, name)) {
				break
			}
		}
	}
	return strings.Join(b.stubs, declarationGap), b.sortedFiles()
}

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func joinPath(owner, name string) string {
	if owner == "" {
		return name
	}
	return owner + pathSeparator + name
}

// rootsOf lists the directories packages are looked up under: the usual ones, the
// source root the test file lies under, and the main source root that goes with a
// test root.
func rootsOf(testFile, packageName string) []string {
	roots := slices.Clone(sourceRoots)
	directory := path.Dir(testFile)
	packageDirectory := strings.ReplaceAll(packageName, pathSeparator, segmentJoin)
	if packageDirectory != "" && strings.HasSuffix(directory, packageDirectory) {
		root := strings.TrimSuffix(strings.TrimSuffix(directory, packageDirectory), segmentJoin)
		roots = append(roots, root, strings.Replace(root, "/test/", "/main/", 1), strings.Replace(root, "test/", "main/", 1))
	} else if packageDirectory == "" {
		roots = append(roots, directory)
	}
	return slices.Compact(roots)
}

func (b *signatureBuilder) release() {
	for _, index := range b.files {
		if index != nil {
			index.Release()
		}
	}
}

func (b *signatureBuilder) sortedFiles() []string {
	var files []string
	for file := range b.read {
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}

// declaration is a top-level declaration of a project file.
type declaration struct {
	file  string
	index *fileIndex
	node  *gotreesitter.Node
}

// addQualified writes the stub of the declaration with the qualified name: a type, a
// top-level function or property, or a member of a type. It reports whether the
// project holds it.
func (b *signatureBuilder) addQualified(qualified string) bool {
	segments := strings.Split(qualified, pathSeparator)
	for length := len(segments); length >= 1; length-- {
		found, ok := b.findTopLevel(strings.Join(segments[:length-1], pathSeparator), segments[length-1])
		if !ok {
			continue
		}
		node, only := found.index.descend(found.node, segments[length:])
		key := found.file + "#" + qualified
		if !b.published[key] {
			b.published[key] = true
			b.read[found.file] = true
			b.stubs = append(b.stubs, found.index.stubDeclaration(node, "", only))
		}
		return true
	}
	return false
}

// findTopLevel looks in the files of a package for the top-level declaration with
// the name.
func (b *signatureBuilder) findTopLevel(packageName, name string) (declaration, bool) {
	directory := strings.ReplaceAll(packageName, pathSeparator, segmentJoin)
	for _, root := range b.roots {
		entries, err := fs.ReadDir(b.project, path.Join(root, directory))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), sourceSuffix) {
				continue
			}
			file := path.Join(root, directory, entry.Name())
			index := b.load(file)
			if index == nil || index.packageName() != packageName {
				continue
			}
			for _, top := range index.childrenOf(index.Root()) {
				if slices.Contains(index.declaredNames(top), name) && !index.hasModifier(top, tokenPrivate) {
					return declaration{file: file, index: index, node: top}, true
				}
			}
		}
	}
	return declaration{}, false
}

// load parses a project file once. A file that cannot be read yields nil.
func (b *signatureBuilder) load(file string) *fileIndex {
	if index, loaded := b.files[file]; loaded {
		return index
	}
	source, err := fs.ReadFile(b.project, file)
	if err != nil {
		b.files[file] = nil
		return nil
	}
	index, err := indexFile(file, source)
	if err != nil {
		b.files[file] = nil
		return nil
	}
	b.files[file] = index
	return index
}

// descend follows the names of the types nested in a declaration; a name that is
// not a nested type is a member, which the stub is restricted to.
func (x *fileIndex) descend(node *gotreesitter.Node, names []string) (*gotreesitter.Node, map[string]bool) {
	for position, name := range names {
		var next *gotreesitter.Node
		for _, inner := range x.childrenOf(x.bodyOf(node)) {
			if x.isType(inner) && x.typeName(inner) == name {
				next = inner
			}
		}
		if next == nil {
			return node, map[string]bool{names[position]: true}
		}
		node = next
	}
	return node, nil
}

// stubDeclaration writes a declaration without bodies, annotations or private
// members; only, when not nil, restricts the members of a type to those it names.
func (x *fileIndex) stubDeclaration(node *gotreesitter.Node, indent string, only map[string]bool) string {
	switch x.Type(node) {
	case typeFunction:
		return indent + x.header(node, x.childOfType(node, typeFunctionBody))
	case typeProperty:
		return indent + x.header(node, x.initializerOf(node))
	case typeClass, typeObject, typeCompanion:
		return x.stubType(node, indent, only)
	}
	return ""
}

// initializerOf is the node where the value of a property starts: its initializer,
// its delegate or its accessors.
func (x *fileIndex) initializerOf(property *gotreesitter.Node) *gotreesitter.Node {
	for _, child := range x.Children(property) {
		switch x.Type(child) {
		case typePropertyDelegate, typeGetter, typeSetter:
			return child
		}
		if x.Text(child) == tokenAssign {
			return child
		}
	}
	return nil
}

func (x *fileIndex) stubType(declaration *gotreesitter.Node, indent string, only map[string]bool) string {
	body := x.bodyOf(declaration)
	if body == nil {
		return indent + x.header(declaration, nil)
	}
	lines := []string{indent + x.header(declaration, body) + stubOpen}
	if constants := x.enumEntries(body); constants != "" && only == nil {
		lines = append(lines, indent+stubIndent+constants)
	}
	for _, member := range x.childrenOf(body) {
		if x.hiddenFromStub(member, only) {
			continue
		}
		if text := x.stubDeclaration(member, indent+stubIndent, nil); text != "" {
			lines = append(lines, text)
		}
	}
	return strings.Join(append(lines, indent+stubClose), "\n")
}

// hiddenFromStub reports whether a member is left out of a stub: it is private, or
// the stub is restricted to other members.
func (x *fileIndex) hiddenFromStub(member *gotreesitter.Node, only map[string]bool) bool {
	if x.hasModifier(member, tokenPrivate) {
		return true
	}
	if only == nil {
		return false
	}
	return !slices.ContainsFunc(x.declaredNames(member), func(name string) bool { return only[name] })
}

func (x *fileIndex) enumEntries(body *gotreesitter.Node) string {
	if x.Type(body) != typeEnumBody {
		return ""
	}
	var names []string
	for _, entry := range x.childrenOf(body) {
		if x.Type(entry) == typeEnumEntry {
			names = append(names, x.Text(x.childOfType(entry, typeSimpleIdentifier)))
		}
	}
	return strings.Join(names, constantJoin)
}

// hasModifier reports whether the modifiers of a declaration hold the word.
func (x *fileIndex) hasModifier(declaration *gotreesitter.Node, word string) bool {
	modifiers := x.childOfType(declaration, typeModifiers)
	return modifiers != nil && slices.Contains(strings.Fields(x.modifierText(modifiers)), word)
}

// modifierText is the modifiers of a declaration without its annotations.
func (x *fileIndex) modifierText(modifiers *gotreesitter.Node) string {
	var words []string
	for _, child := range x.Named(modifiers) {
		if x.Type(child) != typeAnnotation {
			words = append(words, x.Text(child))
		}
	}
	return strings.Join(words, wordJoin)
}

// header is the text of a declaration up to the start of the end node, without
// annotations and comments and on one line; the whole declaration when the end is
// nil.
func (x *fileIndex) header(declaration, end *gotreesitter.Node) string {
	stop := int(declaration.EndByte())
	if end != nil {
		stop = int(end.StartByte())
	}
	edits := append([]textedit.Edit(nil), x.removals...)
	if modifiers := x.childOfType(declaration, typeModifiers); modifiers != nil {
		for _, child := range x.Named(modifiers) {
			if x.Type(child) == typeAnnotation {
				edits = append(edits, textedit.Edit{Span: textedit.Span{Start: int(child.StartByte()), End: int(child.EndByte())}})
			}
		}
	}
	text := textedit.Render(x.Source, textedit.Span{Start: int(declaration.StartByte()), End: stop}, edits)
	return strings.TrimSpace(strings.Join(strings.Fields(text), wordJoin))
}
