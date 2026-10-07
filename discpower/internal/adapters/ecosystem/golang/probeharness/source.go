package probeharness

import (
	"bytes"
	_ "embed"
)

const (
	packageClause     = "package probeharness"
	programClause     = "package main"
	clauseReplacement = 1
)

//go:embed harness.go
var source []byte

// ProgramSource is the harness as the source of a `main` package, to be built
// next to the generated table of operations.
func ProgramSource() []byte {
	return bytes.Replace(source, []byte(packageClause), []byte(programClause), clauseReplacement)
}
