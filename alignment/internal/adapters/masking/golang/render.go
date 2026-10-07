package golang

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"sort"
	"strconv"
	"strings"
)

const (
	// packageClause stands in front of the rendered declarations while they are
	// formatted; it is not part of the output.
	packageClause    = "package neutral"
	declarationBreak = "\n\n"
)

// collectImports returns the imports of the files involved that the
// declarations use, merged, std-lib first, then sorted by path.
func collectImports(decls []*declaration) []importEntry {
	seen := map[importEntry]bool{}
	for _, decl := range decls {
		walkReferences(decl.node, nil, func(base, _ *ast.Ident) {
			if entry, imported := decl.file.imports[base.Name]; imported && base.Obj == nil {
				seen[entry] = true
			}
		})
	}
	entries := make([]importEntry, 0, len(seen))
	for entry := range seen {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if standard := isStandardLibrary(entries[i].path); standard != isStandardLibrary(entries[j].path) {
			return standard
		}
		if entries[i].path != entries[j].path {
			return entries[i].path < entries[j].path
		}
		return entries[i].alias < entries[j].alias
	})
	return entries
}

// isStandardLibrary reports whether an import path is in the standard library: its
// first element has no dot.
func isStandardLibrary(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}

func renderImports(entries []importEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var block strings.Builder
	block.WriteString("import (\n")
	for index, entry := range entries {
		if index > 0 && isStandardLibrary(entries[index-1].path) && !isStandardLibrary(entry.path) {
			block.WriteString("\n")
		}
		block.WriteString("\t")
		if entry.alias != "" {
			block.WriteString(entry.alias + " ")
		}
		block.WriteString(strconv.Quote(entry.path) + "\n")
	}
	block.WriteString(")")
	return block.String()
}

// renderUnit prints the imports and the declarations as one gofmt-formatted file
// body, without package clause, separated by one blank line.
func renderUnit(fset *token.FileSet, imports []importEntry, decls []*declaration) (string, error) {
	var parts []string
	if block := renderImports(imports); block != "" {
		parts = append(parts, block)
	}
	for _, decl := range decls {
		text, err := printNode(fset, decl.node)
		if err != nil {
			return "", err
		}
		parts = append(parts, text)
	}
	source := packageClause + declarationBreak + strings.Join(parts, declarationBreak) + "\n"
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return "", fmt.Errorf("format the masked unit: %w", err)
	}
	tidy := dropBraceBlankLines(formatted)
	return strings.TrimPrefix(string(tidy), packageClause+declarationBreak), nil
}

func printNode(fset *token.FileSet, node ast.Node) (string, error) {
	var buffer bytes.Buffer
	if err := format.Node(&buffer, fset, node); err != nil {
		return "", fmt.Errorf("print a declaration: %w", err)
	}
	return buffer.String(), nil
}
