// Package syntaxtree parses a source file with a tree-sitter grammar. The
// grammars' lexers read a NUL byte as the end of the input, so a NUL a file
// holds, in a string or template literal for instance, is read as a space of
// the same width: every node keeps the offsets it has in the file.
package syntaxtree

import (
	"bytes"

	"github.com/odvcencio/gotreesitter"
)

const (
	nul        = 0x00
	nulReading = ' '
)

// Parse parses the source with the language's grammar.
func Parse(language *gotreesitter.Language, source []byte) (*gotreesitter.Tree, error) {
	return gotreesitter.NewParser(language).Parse(readable(source))
}

// readable is the source with each NUL byte read as a space, or the source
// itself when it holds none.
func readable(source []byte) []byte {
	if bytes.IndexByte(source, nul) < 0 {
		return source
	}
	read := bytes.Clone(source)
	for index, value := range read {
		if value == nul {
			read[index] = nulReading
		}
	}
	return read
}
