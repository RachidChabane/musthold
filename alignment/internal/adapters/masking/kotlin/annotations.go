package kotlin

import (
	"fmt"
	"regexp"
)

// The grammar reads a class that several annotations precede as a sequence of
// expressions. It reads the same annotations written as one bracketed list,
// `@[First Second]`, as the modifiers of the class, so the parser is given the
// source with every such run rewritten that way. The rewrite keeps the length of
// the text and the offset of everything but the first annotation of a run.

const (
	annotationExpression = `@[A-Za-z_][\w.]*(?::[A-Za-z_]\w*)?(?:\((?:[^()"']|"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|\((?:[^()"']|"(?:[^"\\]|\\.)*")*\))*\))?`
	declarationHead      = `(?:(?:public|private|protected|internal|open|abstract|final|sealed|data|inner|enum|annotation|value|companion)\s+)*(?:class|interface|object)\b`
	minimumRunLength     = 2
	listOpen             = "@["
	listClose            = "]"
	blank                = ' '
)

var (
	annotationPattern = regexp.MustCompile(annotationExpression)
	runPattern        = regexp.MustCompile(fmt.Sprintf(`((?:%s\s+){%d,})%s`, annotationExpression, minimumRunLength, declarationHead))
)

// groupedAnnotations returns the source with the annotations that precede a type
// declaration, when there are two or more, written as a bracketed list.
func groupedAnnotations(source []byte) []byte {
	rewritten := append([]byte(nil), source...)
	for _, found := range runPattern.FindAllSubmatchIndex(source, -1) {
		regroup(rewritten, source, found[2], found[3])
	}
	return rewritten
}

// regroup rewrites the run of annotations and the white space that follows each
// of them, which lies between start and end.
func regroup(rewritten, source []byte, start, end int) {
	spans := annotationPattern.FindAllIndex(source[start:end], -1)
	if len(spans) < minimumRunLength {
		return
	}
	first := start + spans[0][0]
	firstEnd := start + spans[0][1]
	// The first annotation takes the bracket and gives up the first character of
	// the white space after it.
	opened := append([]byte(listOpen), source[first+1:firstEnd]...)
	opened = append(opened, source[firstEnd+1:start+spans[1][0]]...)
	copy(rewritten[first:], opened)
	for _, span := range spans[1:] {
		rewritten[start+span[0]] = blank
	}
	// The last annotation takes the closing bracket in the place of the first
	// character of the white space after it.
	lastEnd := start + spans[len(spans)-1][1]
	closed := append([]byte(listClose), source[lastEnd+1:end]...)
	copy(rewritten[lastEnd:], closed)
}
