package kotlin

import (
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/alignment/internal/platform/syntax"
	"github.com/RachidChabane/musthold/alignment/internal/platform/textedit"
)

// Layout of the unit.
const (
	memberIndent   = "    "
	blockGap       = "\n\n"
	lineBreak      = "\n"
	annotationMark = "@"
	classOpen      = "class " + maskedClassName + " {"
	classClose     = "}"
)

// renderer writes the unit of a plan.
type renderer struct {
	*fileIndex
	edits []textedit.Edit
}

// renderUnit writes the imports, the declarations of the file the test reaches and
// one class that holds the members of the classes around the test and the test,
// every declaration under its neutral name, without comments and without the
// annotations that name the test.
func (x *fileIndex) renderUnit(p plan, imports []importDeclaration) string {
	r := renderer{fileIndex: x, edits: x.editsFor(p)}
	importLines := make([]string, len(imports))
	for index, declaration := range imports {
		importLines[index] = declaration.text
	}
	blocks := []string{strings.Join(importLines, lineBreak)}
	for _, chosen := range p.declarations {
		blocks = append(blocks, r.member(chosen.node, ""))
	}
	return textedit.Join(append(blocks, r.class(p)))
}

// editsFor lists every rewrite of the unit: the removals of the file and the
// neutral names.
func (x *fileIndex) editsFor(p plan) []textedit.Edit {
	edits := append([]textedit.Edit(nil), x.removals...)
	for _, f := range p.fragments() {
		for _, use := range f.occurrences {
			neutral, renamed := p.renames[use.name]
			if renamed && (use.inString || use.reaches(p.classes)) {
				edits = append(edits, textedit.Edit{Span: use.span, Replacement: replacement(use, neutral)})
			}
		}
	}
	return edits
}

// replacement is the text that takes the place of a name: a name read into a string
// with `$name` is braced so that the text after it cannot join it.
func replacement(use occurrence, neutral string) string {
	if use.interpolated {
		return "{" + neutral + "}"
	}
	return neutral
}

// class writes the class around the members of the unit.
func (r renderer) class(p plan) string {
	var lines []string
	seen := map[string]bool{}
	for _, kept := range p.annotations {
		text := annotationMark + textedit.Tidy(r.text(syntax.Span(kept.item)))
		if !seen[text] {
			seen[text] = true
			lines = append(lines, text)
		}
	}
	lines = append(lines, classOpen)
	var members []string
	for _, pieces := range p.blocks {
		for _, chosen := range pieces {
			members = append(members, r.member(chosen.node, memberIndent))
		}
	}
	members = append(members, r.member(p.test.function, memberIndent))
	lines = append(lines, strings.Join(members, blockGap), classClose)
	return strings.Join(lines, lineBreak)
}

// member writes a declaration at the indentation.
func (r renderer) member(node *gotreesitter.Node, indentation string) string {
	start := textedit.LineStart(r.Source, int(node.StartByte()))
	text := textedit.DedentAll(r.text(textedit.Span{Start: start, End: int(node.EndByte())}), int(node.StartByte())-start)
	return textedit.Indent(textedit.Tidy(text), indentation)
}

func (r renderer) text(span textedit.Span) string { return textedit.Render(r.Source, span, r.edits) }
