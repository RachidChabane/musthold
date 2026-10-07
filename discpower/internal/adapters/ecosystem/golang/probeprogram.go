package golang

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/golang/probeharness"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

const (
	// probeDirectory names the package of the probe program. It exists only in
	// the overlay of the build, never on disk.
	probeDirectory = "zz_spec_proof_probe"
	tableFileName  = "main.go"
	harnessName    = "harness.go"
	importAlias    = "target"
)

const tableSource = `package main

import (
	"os"
{{range .Imports}}	{{.Alias}} {{printf "%q" .Path}}
{{end}})

var table = []Entry{
{{range .Entries}}	{
		Target:       {{printf "%q" .Target}},
		Func:         {{.Function}},
		Context:      {{.Context}},
		ReturnsError: {{.ReturnsError}},
		Params: []Param{
{{range .Params}}			{Name: {{printf "%q" .Name}}, Kind: {{printf "%q" .Kind}}, Pointer: {{.Pointer}}},
{{end}}		},
		Sentinels: []Sentinel{
{{range .Sentinels}}			{Name: {{printf "%q" .Name}}, Value: {{.Expression}}},
{{end}}		},
	},
{{end}}}

func main() { os.Exit(Main(os.Args[1:], table)) }
`

var tableTemplate = template.Must(template.New("table").Parse(tableSource))

type tableImport struct{ Alias, Path string }

type tableSentinel struct{ Name, Expression string }

type tableEntry struct {
	Target       string
	Function     string
	Context      bool
	ReturnsError bool
	Params       []probeharness.Param
	Sentinels    []tableSentinel
}

type table struct {
	Imports []tableImport
	Entries []tableEntry
}

// programGroup is the operations one probe program serves: the directory the
// program is built in and the operations it calls.
type programGroup struct {
	Dir        string
	Operations []operation
}

// groupOperations splits operations into programs. A program sits in the
// directory that may import every package it calls: the Go rule for `internal`
// packages lets only the subtree of the parent of `internal` import them.
func groupOperations(workspace ecosystem.Workspace, operations []operation) []programGroup {
	roots := make([]string, len(operations))
	for index, operation := range operations {
		roots[index] = visibilityRoot(workspace, operation.Dir)
	}
	order := make([]int, len(operations))
	for index := range order {
		order[index] = index
	}
	slices.SortStableFunc(order, func(a, b int) int { return len(roots[b]) - len(roots[a]) })
	var groups []programGroup
	for _, index := range order {
		position := slices.IndexFunc(groups, func(group programGroup) bool { return within(group.Dir, roots[index]) })
		if position < 0 {
			groups = append(groups, programGroup{Dir: roots[index]})
			position = len(groups) - 1
		}
		groups[position].Operations = append(groups[position].Operations, operations[index])
	}
	return groups
}

// within reports whether dir is root or lies below it.
func within(dir, root string) bool {
	relative, err := filepath.Rel(root, dir)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// renderTable writes the source of the table of operations of a program.
func renderTable(operations []operation) ([]byte, error) {
	var data table
	aliases := map[string]string{}
	for _, operation := range operations {
		alias, known := aliases[operation.Target.ImportPath]
		if !known {
			alias = importAlias + strconv.Itoa(len(aliases))
			aliases[operation.Target.ImportPath] = alias
			data.Imports = append(data.Imports, tableImport{Alias: alias, Path: operation.Target.ImportPath})
		}
		entry := tableEntry{
			Target: operation.Target.Text, Function: functionExpression(alias, operation.Target),
			Context: operation.Context, ReturnsError: operation.ReturnsError, Params: operation.Params,
		}
		for _, name := range operation.Sentinels {
			entry.Sentinels = append(entry.Sentinels, tableSentinel{Name: name, Expression: alias + "." + name})
		}
		data.Entries = append(data.Entries, entry)
	}
	var rendered bytes.Buffer
	if err := tableTemplate.Execute(&rendered, data); err != nil {
		return nil, fmt.Errorf("render the table of operations: %w", err)
	}
	return rendered.Bytes(), nil
}

// functionExpression is the Go expression of the function to call: the function
// itself, or the method value of a zero value of its receiver.
func functionExpression(alias string, target probeTarget) string {
	if target.Receiver == "" {
		return alias + "." + target.Name
	}
	return fmt.Sprintf("func() any { var receiver %s.%s; return receiver.%s }()", alias, target.Receiver, target.Name)
}

// overlayOf is the build overlay that places the table and the harness in the
// probe package under dir without writing them there.
func overlayOf(dir, tableFile, harnessFile string) ([]byte, error) {
	overlay := struct{ Replace map[string]string }{Replace: map[string]string{
		filepath.Join(dir, probeDirectory, tableFileName): tableFile,
		filepath.Join(dir, probeDirectory, harnessName):   harnessFile,
	}}
	return json.Marshal(overlay)
}
