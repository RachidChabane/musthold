package golang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// lineSeparator ends a line of a Go source file.
const lineSeparator = "\n"

// clauseHeader is the header of a case or default clause of a switch, type
// switch or select, written on one line from its keyword to its colon: the
// line, and the column of the colon, counted in bytes from one.
type clauseHeader struct {
	line, colonColumn int
}

// moduleSource is the source of the module's files, as the ownership of a line
// reads it.
type moduleSource interface {
	// line gives the text of a line of a file relative to the module root, and
	// whether it could be read.
	line(file string, line int) (string, bool)
	// clauseHeaders gives the one-line clause headers of a file relative to the
	// module root that no fallthrough enters, in source order; a file that
	// cannot be read or parsed has none.
	clauseHeaders(file string) []clauseHeader
}

// parsedFile is what is kept of a file once read: its lines, and its clause
// headers.
type parsedFile struct {
	lines   []string
	headers []clauseHeader
}

// parseFile reads the lines and the clause headers of Go source.
func parseFile(content []byte) parsedFile {
	return parsedFile{
		lines:   strings.Split(string(content), lineSeparator),
		headers: clauseHeadersOf(content),
	}
}

// sourceReader reads the files of the module, each once, for any number of
// goroutines.
type sourceReader struct {
	root  string
	mutex sync.Mutex
	files map[string]*parsedFile
}

func newSourceReader(root string) *sourceReader {
	return &sourceReader{root: root, files: map[string]*parsedFile{}}
}

// file gives a file relative to the module root, nil when it cannot be read.
func (r *sourceReader) file(name string) *parsedFile {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	file, read := r.files[name]
	if !read {
		if content, err := os.ReadFile(filepath.Join(r.root, filepath.FromSlash(name))); err == nil {
			parsed := parseFile(content)
			file = &parsed
		}
		r.files[name] = file
	}
	return file
}

func (r *sourceReader) line(name string, line int) (string, bool) {
	file := r.file(name)
	if file == nil || line < 1 || line > len(file.lines) {
		return "", false
	}
	return file.lines[line-1], true
}

func (r *sourceReader) clauseHeaders(name string) []clauseHeader {
	if file := r.file(name); file != nil {
		return file.headers
	}
	return nil
}

// clauseHeadersOf finds the clause headers of Go source that fit on one line
// and that no fallthrough of the previous clause enters; source that does not
// parse has none.
func clauseHeadersOf(content []byte) []clauseHeader {
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, "", content, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	var headers []clauseHeader
	ast.Inspect(tree, func(node ast.Node) bool {
		var clauses []ast.Stmt
		switch statement := node.(type) {
		case *ast.SwitchStmt:
			clauses = statement.Body.List
		case *ast.TypeSwitchStmt:
			clauses = statement.Body.List
		case *ast.SelectStmt:
			clauses = statement.Body.List
		default:
			return true
		}
		entered := false
		for _, clause := range clauses {
			keyword, colon, body := clauseParts(clause)
			header, oneLine := oneLineHeader(fset, keyword, colon)
			if oneLine && !entered {
				headers = append(headers, header)
			}
			entered = endsInFallthrough(body)
		}
		return true
	})
	return headers
}

// clauseParts gives the keyword position, the colon position and the body of a
// case clause of a switch or of a comm clause of a select.
func clauseParts(clause ast.Stmt) (keyword, colon token.Pos, body []ast.Stmt) {
	switch clause := clause.(type) {
	case *ast.CaseClause:
		return clause.Case, clause.Colon, clause.Body
	case *ast.CommClause:
		return clause.Case, clause.Colon, clause.Body
	}
	return token.NoPos, token.NoPos, nil
}

// oneLineHeader gives the header whose keyword and colon share a line.
func oneLineHeader(fset *token.FileSet, keyword, colon token.Pos) (clauseHeader, bool) {
	if !keyword.IsValid() || !colon.IsValid() {
		return clauseHeader{}, false
	}
	start, end := fset.PositionFor(keyword, false), fset.PositionFor(colon, false)
	return clauseHeader{line: end.Line, colonColumn: end.Column}, start.Line == end.Line
}

// endsInFallthrough reports whether the last statement of a clause body that
// is not empty is a fallthrough.
func endsInFallthrough(body []ast.Stmt) bool {
	for index := len(body) - 1; index >= 0; index-- {
		statement := body[index]
		for {
			labeled, isLabeled := statement.(*ast.LabeledStmt)
			if !isLabeled {
				break
			}
			statement = labeled.Stmt
		}
		if _, empty := statement.(*ast.EmptyStmt); empty {
			continue
		}
		branch, isBranch := statement.(*ast.BranchStmt)
		return isBranch && branch.Tok == token.FALLTHROUGH
	}
	return false
}
