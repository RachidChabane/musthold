package golang

import (
	"go/scanner"
	"go/token"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
)

const (
	normalizePath    = "unit.go"
	packageClause    = "package unit\n"
	assignmentPrefix = "var "
	tokenSeparator   = "\x1f"
	semicolonText    = ";"
)

// normalizeUnit checks that a unit parses and reduces it to its token stream
// without comments: layout and comments do not appear in the result, and
// automatic and explicit semicolons are the same token. A spec is wrapped in a
// var declaration and a module addition is a whole declaration.
func normalizeUnit(target mutation.TargetKind, unitSource string) (string, error) {
	wrapped := packageClause + unitSource
	if target == mutation.TargetModuleAssignment {
		wrapped = packageClause + assignmentPrefix + unitSource
	}
	if _, err := parseSource(normalizePath, []byte(wrapped)); err != nil {
		return "", err
	}
	return tokenStream(wrapped), nil
}

func tokenStream(text string) string {
	fset := token.NewFileSet()
	file := fset.AddFile(normalizePath, fset.Base(), len(text))
	var lexer scanner.Scanner
	lexer.Init(file, []byte(text), nil, 0)
	var tokens []string
	for {
		_, kind, literal := lexer.Scan()
		if kind == token.EOF {
			return strings.Join(tokens, tokenSeparator)
		}
		if kind == token.SEMICOLON {
			literal = semicolonText
		}
		tokens = append(tokens, kind.String()+tokenSeparator+literal)
	}
}
