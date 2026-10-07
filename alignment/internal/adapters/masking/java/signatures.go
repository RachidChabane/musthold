package java

import (
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/textedit"
)

// Layout of the stubs.
const (
	stubIndent     = "    "
	stubEnd        = ";"
	stubOpen       = " {"
	stubClose      = "}"
	declarationGap = "\n\n"
	sourceSuffix   = ".java"
	segmentJoin    = "/"
	wordJoin       = " "
	constantJoin   = ", "
)

// sourceRoots are the directories a class is looked up under; the empty root is
// the project root.
var sourceRoots = []string{"", "src", "src/main/java", "src/test/java"}

// signatureBuilder reads the project classes the unit refers to and writes their
// declarations without bodies.
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

// buildSignatures returns the stubs of the project classes the unit imports or
// names from its own package, and the project files they were read from, sorted.
func buildSignatures(project fs.FS, query signatureQuery) (string, []string) {
	b := &signatureBuilder{project: project, roots: rootsOf(query.testFile, query.packageName), files: map[string]*fileIndex{}, published: map[string]bool{}, read: map[string]bool{}}
	defer b.release()
	bound := map[string]bool{}
	for _, declaration := range query.imports {
		b.addImport(declaration)
		bound[declaration.simple()] = true
	}
	packages := []string{query.packageName}
	for _, declaration := range query.imports {
		if declaration.wildcard && !declaration.static {
			packages = append(packages, declaration.path)
		}
	}
	for _, name := range sortedNames(query.used) {
		if bound[name] || query.declared[name] || !isTypeName(name) {
			continue
		}
		for _, owner := range packages {
			if b.addClass(joinPath(owner, name), nil) {
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

func isTypeName(name string) bool {
	first, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(first) && strings.ContainsFunc(name, unicode.IsLower)
}

func joinPath(owner, name string) string {
	if owner == "" {
		return name
	}
	return owner + pathSeparator + name
}

// rootsOf lists the directories classes are looked up under: the usual ones, the
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

// addImport writes the stub of the class an import names, restricted to the
// member a static import names.
func (b *signatureBuilder) addImport(declaration importDeclaration) {
	switch {
	case declaration.wildcard && !declaration.static:
		return
	case declaration.static && !declaration.wildcard:
		b.addClass(declaration.owner(), map[string]bool{declaration.simple(): true})
	case declaration.static:
		b.addClass(declaration.path, nil)
	default:
		b.addClass(declaration.path, nil)
	}
}

// addClass writes the stub of the class with the qualified name, or of the
// members of it that only holds. It reports whether the project holds the class.
func (b *signatureBuilder) addClass(qualified string, only map[string]bool) bool {
	segments := strings.Split(qualified, pathSeparator)
	for length := len(segments); length >= 1; length-- {
		file, index := b.find(segments[:length])
		if index == nil {
			continue
		}
		declaration := index.findType(segments[length-1:])
		if declaration == nil {
			return false
		}
		key := file + "#" + qualified + "#" + strings.Join(sortedNames(only), ",")
		if !b.published[key] {
			b.published[key] = true
			b.read[file] = true
			b.stubs = append(b.stubs, index.stubType(declaration, "", only))
		}
		return true
	}
	return false
}

// find looks for the file that declares the top-level class named by the segments.
func (b *signatureBuilder) find(segments []string) (string, *fileIndex) {
	for _, root := range b.roots {
		file := path.Join(append([]string{root}, segments...)...) + sourceSuffix
		if index := b.load(file); index != nil {
			return file, index
		}
	}
	return "", nil
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

// findType finds the top-level type with the first name, and the types nested in it
// with the next ones.
func (x *fileIndex) findType(names []string) *gotreesitter.Node {
	candidates := x.topLevelTypes()
	var found *gotreesitter.Node
	for _, name := range names {
		found = nil
		for _, candidate := range candidates {
			if x.Text(x.Field(candidate, fieldName)) == name {
				found = candidate
				break
			}
		}
		if found == nil {
			return nil
		}
		candidates = nil
		for _, member := range x.memberNodes(found) {
			candidates = append(candidates, member)
		}
	}
	return found
}

// stubType writes a type declaration without bodies, annotations or private
// members; only, when not nil, restricts the members to those it names.
func (x *fileIndex) stubType(declaration *gotreesitter.Node, indent string, only map[string]bool) string {
	interfaceLike := x.Type(declaration) == typeInterface || x.Type(declaration) == typeAnnotationType
	lines := []string{indent + x.header(declaration, x.bodyOf(declaration)) + stubOpen}
	if constants := x.enumConstants(declaration); constants != "" && only == nil {
		lines = append(lines, indent+stubIndent+constants+stubEnd)
	}
	for _, member := range x.memberNodes(declaration) {
		if text := x.stubMember(member, indent+stubIndent, only, interfaceLike); text != "" {
			lines = append(lines, text)
		}
	}
	return strings.Join(append(lines, indent+stubClose), lineBreak)
}

func (x *fileIndex) enumConstants(declaration *gotreesitter.Node) string {
	if x.Type(declaration) != typeEnum {
		return ""
	}
	var names []string
	for _, child := range x.childrenOf(x.bodyOf(declaration)) {
		if x.Type(child) == typeEnumConstant {
			names = append(names, x.Text(x.Field(child, fieldName)))
		}
	}
	return strings.Join(names, constantJoin)
}

func (x *fileIndex) stubMember(member *gotreesitter.Node, indent string, only map[string]bool, interfaceLike bool) string {
	if !interfaceLike && x.hasModifier(member, tokenPrivate) {
		return ""
	}
	if only != nil && !slices.ContainsFunc(x.declaredNames(member), func(name string) bool { return only[name] }) {
		return ""
	}
	switch x.Type(member) {
	case typeMethod, typeConstructor:
		return indent + x.header(member, x.Field(member, fieldBody)) + stubEnd
	case typeField:
		return x.fieldStubs(member, indent)
	case typeClass, typeInterface, typeEnum, typeRecord:
		return x.stubType(member, indent, nil)
	}
	return ""
}

// fieldStubs writes one line per variable a field declaration declares, without
// its value.
func (x *fileIndex) fieldStubs(field *gotreesitter.Node, indent string) string {
	declarators := x.FieldChildren(field, fieldDeclarator)
	if len(declarators) == 0 {
		return ""
	}
	prefix := x.header(field, declarators[0])
	lines := make([]string, len(declarators))
	for index, declarator := range declarators {
		lines[index] = indent + prefix + wordJoin + x.Text(x.Field(declarator, fieldName)) + stubEnd
	}
	return strings.Join(lines, lineBreak)
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
	for _, annotation := range x.annotationsOf(declaration) {
		edits = append(edits, textedit.Edit{Span: textedit.Span{Start: int(annotation.StartByte()), End: int(annotation.EndByte())}})
	}
	text := textedit.Render(x.Source, textedit.Span{Start: int(declaration.StartByte()), End: stop}, edits)
	return strings.TrimSuffix(strings.Join(strings.Fields(text), wordJoin), stubEnd)
}
