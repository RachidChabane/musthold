package node

import (
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"

	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
)

const (
	fileURLScheme = "file"
	// wrapperSuffix closes the async arrow Vitest's module runner wraps a
	// module in, after the transformed code.
	wrapperSuffix   = "\n}}"
	parentSegment   = ".."
	dependencyDir   = "node_modules"
	lineFeed        = '\n'
	openingBrace    = '{'
	closingBrace    = '}'
	noLine          = 0
	firstLineNumber = 1
)

// geometry maps each UTF-16 offset of a module's generated code to the line of
// a project file it holds code of: a character other than whitespace or a
// brace, mapped by the module's source map, or, for a module with no map whose
// code is its file byte for byte, by its own position. Every other offset maps
// to no line.
type geometry struct {
	// lineOf holds, per offset, one more than the index of its line in lines,
	// noLine for none.
	lineOf []int32
	lines  []vacuity.Line
}

// at is the line an offset holds code of.
func (g geometry) at(offset int) (vacuity.Line, bool) {
	if offset < 0 || offset >= len(g.lineOf) || g.lineOf[offset] == noLine {
		return vacuity.Line{}, false
	}
	return g.lines[g.lineOf[offset]-1], true
}

// projectFiles reads the files of the workspace a geometry is checked
// against, and resolves the sources a source map names.
type projectFiles struct {
	// roots are the absolute forms of the workspace directory, its own and
	// with symbolic links resolved.
	roots []string
	read  func(file string) ([]byte, bool)
}

// relative is the project path of a source a map names, relative to the
// module's own file: false for a source outside the project or installed in
// it.
func (p projectFiles) relative(moduleFile, sourceRoot, source string) (string, bool) {
	if strings.HasPrefix(source, fileURLScheme+"://") || path.IsAbs(source) {
		return p.absolute(source)
	}
	if path.IsAbs(sourceRoot) {
		return p.underRoot(path.Join(sourceRoot, source))
	}
	if strings.Contains(source, ":") {
		return "", false
	}
	return projectPath(path.Clean(path.Join(path.Dir(moduleFile), sourceRoot, source)))
}

// absolute is the project path of a file URL or an absolute path; false, with
// an empty path, for any other name.
func (p projectFiles) absolute(name string) (string, bool) {
	switch {
	case strings.HasPrefix(name, fileURLScheme+"://"):
		parsed, err := url.Parse(name)
		if err != nil {
			return "", false
		}
		return p.underRoot(parsed.Path)
	case path.IsAbs(name):
		return p.underRoot(name)
	}
	return "", false
}

func (p projectFiles) underRoot(absolute string) (string, bool) {
	for _, root := range p.roots {
		if rest, found := strings.CutPrefix(path.Clean(absolute), strings.TrimSuffix(root, "/")+"/"); found {
			return projectPath(rest)
		}
	}
	return "", false
}

// projectPath keeps a relative path inside the project and outside its
// installed dependencies.
func projectPath(relative string) (string, bool) {
	segments := strings.Split(relative, "/")
	if relative == "" || segments[0] == parentSegment || strings.Contains("/"+relative+"/", "/"+dependencyDir+"/") {
		return "", false
	}
	return relative, true
}

// geometries computes the geometry of each module once, by its file and its
// generated code.
type geometries struct {
	files projectFiles
	mutex sync.Mutex
	known map[string]geometry
}

func newGeometries(files projectFiles) *geometries {
	return &geometries{files: files, known: map[string]geometry{}}
}

func (g *geometries) of(script scriptRecord) geometry {
	key := script.file + "\x00" + script.sourceMap + "\x00" + script.source
	g.mutex.Lock()
	defer g.mutex.Unlock()
	if known, found := g.known[key]; found {
		return known
	}
	computed := computeGeometry(script, g.files)
	g.known[key] = computed
	return computed
}

// computeGeometry maps the offsets of a module's generated code: through its
// source map, or, with none, by identity when the code after the runner's
// wrapper is the file's content.
func computeGeometry(script scriptRecord, files projectFiles) geometry {
	code := utf16.Encode([]rune(script.source))
	result := geometry{lineOf: make([]int32, len(code))}
	if len(code) == 0 || script.prefix > len(code) {
		return result
	}
	end := len(code)
	if script.prefix > 0 && strings.HasSuffix(script.source, wrapperSuffix) {
		end -= len(wrapperSuffix)
	}
	locate, ok := locator(script, code[script.prefix:end], files)
	if !ok {
		return result
	}
	index := map[vacuity.Line]int32{}
	line, column := 0, 0
	for offset := script.prefix; offset < end; offset++ {
		unit := code[offset]
		if unit == lineFeed {
			line, column = line+1, 0
			continue
		}
		if holdsCode(unit) {
			if target, mapped := locate(line, column); mapped {
				position, known := index[target]
				if !known {
					result.lines = append(result.lines, target)
					position = int32(len(result.lines))
					index[target] = position
				}
				result.lineOf[offset] = position
			}
		}
		column++
	}
	return result
}

// holdsCode reports whether a code unit is a character other than whitespace
// or a brace.
func holdsCode(unit uint16) bool {
	if unit == openingBrace || unit == closingBrace {
		return false
	}
	return !unicode.IsSpace(rune(unit))
}

// locator maps a generated line and column, both zero-based and counted from
// the start of the transformed code, to a project line: through the source
// map, or by identity for a module with no map that is its file byte for byte.
func locator(script scriptRecord, code []uint16, files projectFiles) (func(line, column int) (vacuity.Line, bool), bool) {
	if script.sourceMap == "" {
		content, read := files.read(script.file)
		if !read || !slices.Equal(code, utf16.Encode([]rune(string(content)))) {
			return nil, false
		}
		return func(line, _ int) (vacuity.Line, bool) {
			return vacuity.Line{File: script.file, Line: line + firstLineNumber}, true
		}, true
	}
	decoded, err := parseSourceMap(script.sourceMap)
	if err != nil {
		return nil, false
	}
	sources := make([]string, len(decoded.sources))
	resolved := make([]bool, len(decoded.sources))
	for index, source := range decoded.sources {
		sources[index], resolved[index] = files.relative(script.file, decoded.sourceRoot, source)
	}
	return func(line, column int) (vacuity.Line, bool) {
		source, sourceLine, mapped := decoded.lookup(line, column)
		if !mapped || !resolved[source] {
			return vacuity.Line{}, false
		}
		return vacuity.Line{File: sources[source], Line: sourceLine + firstLineNumber}, true
	}, true
}
