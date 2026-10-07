package golang

import (
	"bytes"
	"go/scanner"
	"go/token"
	"strings"
)

// dropBraceBlankLines removes the blank lines that directly follow an opening
// brace or precede a closing brace, which the removal of statements leaves in a
// body. Blank lines inside literals are never touched: the scan works on tokens.
func dropBraceBlankLines(source []byte) []byte {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(source))
	var lexer scanner.Scanner
	lexer.Init(file, source, nil, scanner.ScanComments)
	dropped := map[int]bool{}
	previous, previousEnd := token.ILLEGAL, 0
	for {
		position, tok, literal := lexer.Scan()
		if tok == token.EOF {
			break
		}
		line := file.Line(position)
		if previous == token.LBRACE || tok == token.RBRACE {
			for blank := previousEnd + 1; blank < line; blank++ {
				dropped[blank] = true
			}
		}
		previous, previousEnd = tok, line+strings.Count(literal, "\n")
	}
	return withoutLines(source, dropped)
}

func withoutLines(source []byte, dropped map[int]bool) []byte {
	var kept [][]byte
	for index, line := range bytes.Split(source, []byte("\n")) {
		if !dropped[index+1] {
			kept = append(kept, line)
		}
	}
	return bytes.Join(kept, []byte("\n"))
}
