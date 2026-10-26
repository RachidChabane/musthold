package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/discpower/internal/platform/witnesskit"
)

var _ ports.Witnessing = (*Strategy)(nil)

const (
	witnessLanguage = "Go, the testing package"
	// witnessConventionsFormat tells a writer how a Go witness is written; the
	// package named is the one the spec mentions most. It reads no test.
	witnessConventionsFormat = "Each witness is `func <name>(t *testing.T)` in a test file of package %s (directory %s), " +
		"the package of the code under test: its identifiers, exported or not, and the helpers of its test files are in scope without a qualifier. " +
		"In \"imports\", list each import of the file as a quoted import path, one per line, \"testing\" included, and use every one of them. " +
		"Declare nothing at the top level besides the test function. A property draws its inputs from `rand.New(rand.NewPCG(<constant>, <constant>))` of " +
		"`math/rand/v2`, never from the global source or the clock."
	witnessFilePrefix    = "zz_musthold_"
	witnessFileSuffix    = "_test.go"
	witnessTestPrefix    = "Test_"
	funcKeyword          = "func"
	witnessDigestLength  = 12
	importPathSeparator  = "/"
	interfaceIndentation = "\t"
	testingImport        = "testing"
)

// testFunctionPrefixes name the functions of a test file the go tool runs, which
// are no helpers.
var testFunctionPrefixes = []string{"Test", "Benchmark", "Example", "Fuzz"}

// Witnesses is the witness host of Go modules.
func (s *Strategy) Witnesses() ports.WitnessHost { return witnessHost{strategy: s} }

type witnessHost struct {
	strategy *Strategy
}

// goPackage is one non-test package of the module.
type goPackage struct {
	dir        string
	importPath string
	name       string
	entries    []witnesskit.Entry
}

// Brief lists the exported declarations of the module's packages the spec
// mentions, without bodies, the helpers of every test file, and the package the
// witnesses belong to: the one the spec mentions most.
func (h witnessHost) Brief(project fs.FS, _ []mutation.SourceFile, specText string) (ports.WitnessBrief, error) {
	packages, err := modulePackages(project)
	if err != nil {
		return ports.WitnessBrief{}, err
	}
	mentioned := witnesskit.Mentions(specText)
	var entries []witnesskit.Entry
	for _, pkg := range packages {
		entries = append(entries, pkg.entries...)
	}
	helpers, err := h.helpers(project)
	if err != nil {
		return ports.WitnessBrief{}, err
	}
	target := targetPackage(packages, mentioned)
	h.strategy.witnessDir = target.dir
	return ports.WitnessBrief{
		Language: witnessLanguage, Conventions: fmt.Sprintf(witnessConventionsFormat, target.name, target.dir),
		Interface: witnesskit.Choose(entries, mentioned), Helpers: helpers,
	}, nil
}

// targetPackage is the package whose declarations the spec mentions most, the
// first by directory on a tie, or the first package when it mentions none.
func targetPackage(packages []goPackage, mentioned func(string) bool) goPackage {
	best, bestCount := goPackage{dir: rootDirectory}, -1
	for _, pkg := range packages {
		count := 0
		for _, entry := range pkg.entries {
			if slices.ContainsFunc(entry.Names, mentioned) {
				count++
			}
		}
		if count > bestCount {
			best, bestCount = pkg, count
		}
	}
	return best
}

// modulePackages parses the non-test Go files of the module, by directory.
func modulePackages(project fs.FS) ([]goPackage, error) {
	module, err := modulePath(project)
	if err != nil {
		return nil, err
	}
	byDir := map[string]*goPackage{}
	var dirs []string
	err = fs.WalkDir(project, rootDirectory, func(file string, entry fs.DirEntry, walkErr error) error {
		switch {
		case walkErr != nil:
			return walkErr
		case entry.IsDir() && file != rootDirectory && (entry.Name() == vendorDir || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "testdata"):
			return fs.SkipDir
		case entry.IsDir() || path.Ext(file) != goSuffix || strings.HasSuffix(file, witnessFileSuffix):
			return nil
		}
		content, err := fs.ReadFile(project, file)
		if err != nil {
			return err
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, content, parser.SkipObjectResolution)
		if err != nil {
			return nil
		}
		dir := path.Dir(file)
		pkg, known := byDir[dir]
		if !known {
			pkg = &goPackage{dir: dir, importPath: importPathOf(module, dir), name: parsed.Name.Name}
			byDir[dir] = pkg
			dirs = append(dirs, dir)
		}
		pkg.entries = append(pkg.entries, exportedEntries(pkg.importPath, parsed)...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read the packages of the module: %w", err)
	}
	slices.Sort(dirs)
	packages := make([]goPackage, len(dirs))
	for index, dir := range dirs {
		packages[index] = *byDir[dir]
	}
	return packages, nil
}

func modulePath(project fs.FS) (string, error) {
	content, err := fs.ReadFile(project, moduleFile)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", moduleFile, err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == moduleDirective {
			if unquoted, err := strconv.Unquote(fields[1]); err == nil {
				return unquoted, nil
			}
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("%s names no module", moduleFile)
}

const moduleDirective = "module"

func importPathOf(module, dir string) string {
	if dir == rootDirectory {
		return module
	}
	return module + importPathSeparator + dir
}

// exportedEntries renders the exported declarations of a file without bodies:
// functions and methods by their signature, types in full, constants and
// variables by name and type.
func exportedEntries(importPath string, file *ast.File) []witnesskit.Entry {
	var entries []witnesskit.Entry
	for _, declaration := range file.Decls {
		switch decl := declaration.(type) {
		case *ast.FuncDecl:
			if !decl.Name.IsExported() || (decl.Recv != nil && !exportedReceiver(decl.Recv)) {
				continue
			}
			names := []string{decl.Name.Name}
			if decl.Recv != nil {
				names = append(names, receiverName(decl.Recv))
			}
			header := *decl
			header.Body, header.Doc = nil, nil
			entries = append(entries, witnesskit.Entry{Names: names, Signature: importPath + ": " + render(&header)})
		case *ast.GenDecl:
			entries = append(entries, genEntries(importPath, decl)...)
		}
	}
	return entries
}

func genEntries(importPath string, decl *ast.GenDecl) []witnesskit.Entry {
	var entries []witnesskit.Entry
	for _, spec := range decl.Specs {
		switch item := spec.(type) {
		case *ast.TypeSpec:
			if item.Name.IsExported() {
				entries = append(entries, witnesskit.Entry{Names: []string{item.Name.Name}, Signature: importPath + ": type " + render(item)})
			}
		case *ast.ValueSpec:
			for _, name := range item.Names {
				if !name.IsExported() {
					continue
				}
				signature := importPath + ": " + decl.Tok.String() + " " + name.Name
				if item.Type != nil {
					signature += " " + render(item.Type)
				}
				entries = append(entries, witnesskit.Entry{Names: []string{name.Name}, Signature: signature})
			}
		}
	}
	return entries
}

func exportedReceiver(receiver *ast.FieldList) bool {
	name := receiverName(receiver)
	return name != "" && unicode.IsUpper([]rune(name)[0])
}

func receiverName(receiver *ast.FieldList) string {
	if receiver == nil || len(receiver.List) == 0 {
		return ""
	}
	expression := receiver.List[0].Type
	for {
		switch typed := expression.(type) {
		case *ast.StarExpr:
			expression = typed.X
		case *ast.IndexExpr:
			expression = typed.X
		case *ast.IndexListExpr:
			expression = typed.X
		case *ast.Ident:
			return typed.Name
		default:
			return ""
		}
	}
}

func render(node any) string {
	var out bytes.Buffer
	if err := printer.Fprint(&out, token.NewFileSet(), node); err != nil {
		return ""
	}
	return strings.ReplaceAll(out.String(), "\n", "\n"+interfaceIndentation)
}

// helpers lists the signatures of the functions of every test file that the go
// tool does not run.
func (h witnessHost) helpers(project fs.FS) ([]string, error) {
	files, err := witnesskit.TestFiles(project, h.strategy.Layout(), nil)
	if err != nil {
		return nil, err
	}
	module, err := modulePath(project)
	if err != nil {
		return nil, err
	}
	var helpers []string
	for _, file := range files {
		content, err := fs.ReadFile(project, file)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", file, err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, content, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		for _, declaration := range parsed.Decls {
			function, isFunction := declaration.(*ast.FuncDecl)
			if !isFunction || runByTheGoTool(function.Name.Name) {
				continue
			}
			header := *function
			header.Body, header.Doc = nil, nil
			helpers = append(helpers, importPathOf(module, path.Dir(file))+" ("+parsed.Name.Name+"): "+render(&header))
		}
	}
	return helpers, nil
}

func runByTheGoTool(name string) bool {
	return slices.ContainsFunc(testFunctionPrefixes, func(prefix string) bool { return strings.HasPrefix(name, prefix) })
}

// Layout writes a set as one test file of the witnesses' package, each witness
// renamed after its short id.
func (h witnessHost) Layout(project fs.FS, set witness.Set) (ports.WitnessFile, error) {
	dir := h.strategy.witnessDir
	if dir == "" {
		dir = rootDirectory
	}
	module, err := modulePath(project)
	if err != nil {
		return ports.WitnessFile{}, err
	}
	name, err := packageName(project, dir)
	if err != nil {
		return ports.WitnessFile{}, err
	}
	file := path.Join(dir, witnessFilePrefix+set.Digest()[:witnessDigestLength]+witnessFileSuffix)
	var content strings.Builder
	fmt.Fprintf(&content, "package %s\n\nimport (\n", name)
	for _, line := range importLines(set.Imports) {
		fmt.Fprintf(&content, "\t%s\n", line)
	}
	content.WriteString(")\n")
	tests := map[string]ecosystem.BoundTest{}
	classname := importPathOf(module, dir)
	for _, w := range set.Witnesses {
		if !declaresOnly(w.Code, w.Name) {
			continue
		}
		renamed := witnesskit.RenamedName(witnessTestPrefix, w.ShortID())
		code, declared := witnesskit.Rename(w.Code, witnesskit.Declaration(funcKeyword, w.Name), renamed)
		if !declared {
			continue
		}
		content.WriteString("\n")
		line := witnesskit.LineOf(content.String(), content.Len())
		content.WriteString(strings.TrimRight(code, "\n") + "\n")
		tests[w.ID] = ecosystem.NewBoundTest(classname, renamed, file, line)
	}
	return ports.WitnessFile{Path: file, Content: []byte(content.String()), Tests: tests}, nil
}

// declaresOnly reports whether a witness's code parses as exactly one top-level
// declaration: the function it is named after, with no receiver. A witness that
// declares anything else could collide with another set's in the package.
func declaresOnly(code, name string) bool {
	parsed, err := parser.ParseFile(token.NewFileSet(), "", witnessPackageClause+code, parser.SkipObjectResolution)
	if err != nil || len(parsed.Decls) != 1 {
		return false
	}
	function, isFunction := parsed.Decls[0].(*ast.FuncDecl)
	return isFunction && function.Recv == nil && function.Name.Name == name
}

// witnessPackageClause lets a witness's code parse on its own.
const witnessPackageClause = "package witness\n\n"

// importLines reads the imports a writer listed, one per line, quoting a bare
// path, and adds "testing" when it is missing.
func importLines(imports string) []string {
	var lines []string
	hasTesting := false
	for _, raw := range strings.Split(imports, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "import"), ";"))
		line = strings.Trim(line, "()")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.Contains(line, "\"") {
			line = strconv.Quote(line)
		}
		hasTesting = hasTesting || strings.HasSuffix(line, strconv.Quote(testingImport))
		lines = append(lines, line)
	}
	if !hasTesting {
		lines = append([]string{strconv.Quote(testingImport)}, lines...)
	}
	return lines
}

// packageName reads the package clause of the non-test files of a directory.
func packageName(project fs.FS, dir string) (string, error) {
	entries, err := fs.ReadDir(project, dir)
	if err != nil {
		return "", fmt.Errorf("read the package directory %q: %w", dir, err)
	}
	for _, entry := range entries {
		file := path.Join(dir, entry.Name())
		if entry.IsDir() || path.Ext(file) != goSuffix || strings.HasSuffix(file, witnessFileSuffix) {
			continue
		}
		content, err := fs.ReadFile(project, file)
		if err != nil {
			return "", err
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, content, parser.PackageClauseOnly)
		if err == nil {
			return parsed.Name.Name, nil
		}
	}
	return "", fmt.Errorf("no Go package in %q", dir)
}
