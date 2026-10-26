package node

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/platform/witnesskit"
)

const (
	qualifiedSeparator = "."
	exportKeyword      = "export"
	exportDefault      = "export default"
	privateMemberMark  = "_"
	hashMemberMark     = "#"
	blankLine          = "\n\n"
	signatureSeparator = ": "
	memberOfFormat     = "%s (member of class %s)"

	kindClass     = "class"
	kindInterface = "interface"
	kindEnum      = "enum"
	kindType      = "type"
)

var (
	whitespaceRun = regexp.MustCompile(`\s+`)
	hiddenMember  = regexp.MustCompile(`\b(?:private|protected)\b`)
	esmExport     = regexp.MustCompile(`(?m)^export\b`)
	// exportedDeclaration finds the exported declarations the locator does not
	// report: variables, classes and types.
	exportedDeclaration = regexp.MustCompile(`(?m)^export[ \t]+(?:declare[ \t]+)?(?:default[ \t]+)?(?:abstract[ \t]+)?(const|let|var|class|interface|type|enum)[ \t]+([A-Za-z_$][\w$]*)`)
	// arrowFunction matches what a variable initialized with an arrow function
	// holds before its body: the parameters and the return type, then `=>`.
	arrowFunction = regexp.MustCompile(`(?s)^(?:async[ \t]*)?(?:\((?:[^()]|\([^()]*\))*\)|[A-Za-z_$][\w$]*)[ \t]*(?::[^=]*?)?[ \t]*=>`)
)

// located is a signature and where its declaration starts, to keep source order.
type located struct {
	offset int
	entry  witnesskit.Entry
}

// scriptFile is one source or test file whose public signatures are listed.
type scriptFile struct {
	specifier string
	content   []byte
	text      string
	// strict requires the export keyword; otherwise a file without a single ES
	// export is a CommonJS module and every top-level function is public.
	strict          bool
	esm             bool
	exportedClasses map[string]bool
}

// entries lists the public signatures of a file: the exported functions, the
// public methods of exported classes, and the exported variables, classes and
// types, each without a body. exportedOnly demands the export keyword.
func (h witnessHost) entries(file string, content []byte, directory string, exportedOnly bool) ([]witnesskit.Entry, error) {
	definitions, err := h.strategy.deps.Locator.Locate(file, content)
	if err != nil {
		return nil, fmt.Errorf("locate the definitions of %q: %w", file, err)
	}
	text := string(content)
	script := scriptFile{
		specifier: importSpecifier(directory, file), content: content, text: text,
		strict: exportedOnly, esm: esmExport.MatchString(text), exportedClasses: map[string]bool{},
	}
	declarations := exportedDeclaration.FindAllStringSubmatchIndex(text, -1)
	for _, match := range declarations {
		if text[match[2]:match[3]] == kindClass {
			script.exportedClasses[text[match[4]:match[5]]] = true
		}
	}
	var found []located
	for _, definition := range definitions {
		item, ok, err := script.definitionEntry(definition)
		if err != nil {
			return nil, err
		}
		if ok {
			found = append(found, item)
		}
	}
	for _, match := range declarations {
		found = append(found, script.declarationEntry(text[match[2]:match[3]], text[match[4]:match[5]], match[0]))
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].offset < found[j].offset })
	entries := make([]witnesskit.Entry, len(found))
	for index, item := range found {
		entries[index] = item.entry
	}
	return entries, nil
}

func (f scriptFile) public() bool { return f.esm || f.strict }

// definitionEntry renders a function or a method of the locator, when it is
// public.
func (f scriptFile) definitionEntry(definition ecosystem.Definition) (located, bool, error) {
	parts := strings.Split(definition.Qualname, qualifiedSeparator)
	source, err := definition.Span.Slice(f.content)
	if err != nil {
		return located{}, false, err
	}
	header := squash(witnesskit.Header(string(source), '{'))
	signature := ""
	switch {
	case definition.Kind == ecosystem.KindMethod && len(parts) == 2:
		owner, name := parts[0], parts[1]
		if (!f.exportedClasses[owner] && f.public()) || isHiddenMember(name, header) {
			return located{}, false, nil
		}
		signature = fmt.Sprintf(memberOfFormat, header, owner)
	case definition.Kind == ecosystem.KindFunction && len(parts) == 1:
		prefix := f.exportPrefix(definition.Span.Start)
		if prefix == "" && f.public() {
			return located{}, false, nil
		}
		signature = strings.TrimSpace(prefix + " " + header)
	default:
		return located{}, false, nil
	}
	return located{offset: definition.Span.Start, entry: witnesskit.Entry{Names: parts, Signature: f.specifier + signatureSeparator + signature}}, true, nil
}

// exportPrefix is `export` or `export default` when the text before a
// definition on its line says so.
func (f scriptFile) exportPrefix(start int) string {
	lineStart := strings.LastIndex(f.text[:start], "\n") + 1
	before := strings.TrimSpace(f.text[lineStart:start])
	if before == exportKeyword || before == exportDefault {
		return before
	}
	return ""
}

func isHiddenMember(name, header string) bool {
	if strings.HasPrefix(name, privateMemberMark) || strings.HasPrefix(name, hashMemberMark) {
		return true
	}
	opening := strings.IndexByte(header, '(')
	if opening < 0 {
		opening = len(header)
	}
	return hiddenMember.MatchString(header[:opening])
}

// declarationEntry renders an exported variable, class or type by its header.
func (f scriptFile) declarationEntry(kind, name string, start int) located {
	rest := f.text[start:]
	var signature string
	switch kind {
	case kindClass:
		signature = squash(witnesskit.Header(rest, '{'))
	case kindInterface, kindEnum:
		signature = blockDeclaration(rest)
	case kindType:
		signature = squash(witnesskit.Header(untilBlankLine(rest), ';'))
	default:
		signature = variableHeader(rest)
	}
	return located{offset: start, entry: witnesskit.Entry{Names: []string{name}, Signature: f.specifier + signatureSeparator + signature}}
}

// variableHeader is a variable's declaration without its value; for an arrow
// function it keeps the parameters and the return type.
func variableHeader(declaration string) string {
	declaration = untilBlankLine(declaration)
	head := witnesskit.Header(declaration, '=')
	after := strings.TrimLeft(declaration[len(head):], " \t")
	if strings.HasPrefix(after, "=") {
		if arrow := arrowFunction.FindString(strings.TrimLeft(after[1:], " \t\n")); arrow != "" {
			return squash(head + " = " + arrow)
		}
	}
	return squash(witnesskit.Header(head, ';'))
}

// blockDeclaration is a declaration through the brace that closes its block.
func blockDeclaration(declaration string) string {
	depth := 0
	for index := strings.IndexByte(declaration, '{'); index >= 0 && index < len(declaration); index++ {
		switch declaration[index] {
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return strings.TrimSpace(declaration[:index+1])
			}
		}
	}
	return squash(witnesskit.Header(untilBlankLine(declaration), ';'))
}

func untilBlankLine(text string) string {
	if end := strings.Index(text, blankLine); end >= 0 {
		return text[:end]
	}
	return text
}

func squash(text string) string { return whitespaceRun.ReplaceAllString(strings.TrimSpace(text), " ") }
