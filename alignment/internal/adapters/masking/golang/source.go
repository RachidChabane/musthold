package golang

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"regexp"
	"strconv"
	"strings"
)

const (
	goFileSuffix          = ".go"
	testFileSuffix        = "_test.go"
	externalPackageSuffix = "_test"
	blankImportName       = "_"
	dotImportName         = "."
	// parseMode excludes comments, so that none of them ever reaches the output.
	parseMode parser.Mode = 0
)

// versionElementPattern matches an import path element that is only a major
// version, such as `v2`.
var versionElementPattern = regexp.MustCompile(`^v[0-9]+$`)

// versionSuffixPattern matches the `.v3` suffix of a gopkg.in element.
var versionSuffixPattern = regexp.MustCompile(`\.v[0-9]+$`)

// importEntry is one import of a file; the alias is empty when implicit.
type importEntry struct {
	alias string
	path  string
}

func (e importEntry) localName() string {
	if e.alias != "" {
		return e.alias
	}
	return packageName(e.path)
}

// packageName is the package name an import path stands for: its last element,
// without a major-version element or suffix.
func packageName(importPath string) string {
	elements := strings.Split(importPath, "/")
	last := elements[len(elements)-1]
	if versionElementPattern.MatchString(last) && len(elements) > 1 {
		last = elements[len(elements)-2]
	}
	return versionSuffixPattern.ReplaceAllString(last, "")
}

// sourceFile is a parsed Go file of the project.
type sourceFile struct {
	path    string
	syntax  *ast.File
	imports map[string]importEntry
}

func parseSource(fset *token.FileSet, project fs.FS, filePath string) (*sourceFile, error) {
	data, err := fs.ReadFile(project, filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}
	syntax, err := parser.ParseFile(fset, filePath, data, parseMode)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}
	return &sourceFile{path: filePath, syntax: syntax, imports: importsOf(syntax)}, nil
}

func importsOf(syntax *ast.File) map[string]importEntry {
	imports := map[string]importEntry{}
	for _, spec := range syntax.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		entry := importEntry{path: importPath}
		if spec.Name != nil {
			entry.alias = spec.Name.Name
		}
		if entry.alias == blankImportName || entry.alias == dotImportName {
			continue
		}
		imports[entry.localName()] = entry
	}
	return imports
}

// parseDirectory parses the files of a project directory the filter accepts, in
// file name order.
func parseDirectory(fset *token.FileSet, project fs.FS, dir string, accept func(name string) bool) ([]*sourceFile, error) {
	entries, err := fs.ReadDir(project, dir)
	if err != nil {
		return nil, fmt.Errorf("read directory %s: %w", dir, err)
	}
	var files []*sourceFile
	for _, entry := range entries {
		if entry.IsDir() || !accept(entry.Name()) {
			continue
		}
		file, err := parseSource(fset, project, path.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func isTestFile(name string) bool { return strings.HasSuffix(name, testFileSuffix) }

func isCodeFile(name string) bool {
	return strings.HasSuffix(name, goFileSuffix) && !isTestFile(name)
}

// testPackage is the set of test files that declare the package of a test file.
type testPackage struct {
	dir    string
	name   string
	files  []*sourceFile
	target *sourceFile
}

// isExternal reports whether the package is an external `_test` package.
func (p *testPackage) isExternal() bool { return strings.HasSuffix(p.name, externalPackageSuffix) }

func loadTestPackage(fset *token.FileSet, project fs.FS, testFile string) (*testPackage, error) {
	dir, file := directoryOf(testFile), path.Clean(testFile)
	candidates, err := parseDirectory(fset, project, dir, isTestFile)
	if err != nil {
		return nil, err
	}
	pkg := &testPackage{dir: dir}
	for _, candidate := range candidates {
		if candidate.path == file {
			pkg.target, pkg.name = candidate, candidate.syntax.Name.Name
		}
	}
	if pkg.target == nil {
		return nil, fmt.Errorf("read %s: %w", testFile, fs.ErrNotExist)
	}
	for _, candidate := range candidates {
		if candidate.syntax.Name.Name == pkg.name {
			pkg.files = append(pkg.files, candidate)
		}
	}
	return pkg, nil
}
