package javascript

import (
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/alignment/internal/platform/syntax"
	"github.com/RachidChabane/musthold/alignment/internal/platform/textedit"
)

const (
	blockIndent   = "  "
	blockGap      = "\n\n"
	lineBreak     = "\n"
	propertyJoint = ": "
)

// renderer writes the unit of a plan.
type renderer struct {
	*fileIndex
	plan  plan
	edits []textedit.Edit
}

// renderUnit writes the blocks of the plan from the module inwards, with every
// title neutral and every declaration under its neutral name, without comments.
// The imports come first, reduced to the names the unit uses.
func (x *fileIndex) renderUnit(p plan, imports []importStatement) string {
	r := renderer{fileIndex: x, plan: p, edits: x.editsFor(p)}
	lines := make([]string, len(imports))
	for index, statement := range imports {
		lines[index] = statement.String()
	}
	return textedit.Join([]string{strings.Join(lines, lineBreak), r.block(0)})
}

// editsFor lists every rewrite of the unit: the removal of the comments, the
// neutral names and the neutral titles.
func (x *fileIndex) editsFor(p plan) []textedit.Edit {
	edits := append([]textedit.Edit(nil), x.comments...)
	for _, f := range p.fragments() {
		for _, use := range f.occurrences {
			if neutral, renamed := p.renames[use.name]; renamed {
				edits = append(edits, textedit.Edit{Span: use.span, Replacement: renaming(use, neutral)})
			}
		}
	}
	for _, enclosing := range p.test.suites {
		edits = append(edits, x.titleEdit(enclosing.call))
	}
	return append(edits, x.titleEdit(p.test.test))
}

// renaming is the text that replaces an identifier: a shorthand property keeps its
// key.
func renaming(use occurrence, neutral string) string {
	if use.shorthand {
		return use.name + propertyJoint + neutral
	}
	return neutral
}

func (x *fileIndex) titleEdit(c call) textedit.Edit {
	return textedit.Edit{Span: syntax.Span(c.title), Replacement: neutralTitle}
}

// block writes the pieces of a depth and what is nested in them: the next suite
// around the test, or the test itself.
func (r renderer) block(depth int) string {
	var parts []string
	for _, chosen := range r.plan.blocks[depth] {
		parts = append(parts, r.statement(chosen.member.statement))
	}
	if depth == len(r.plan.test.suites) {
		parts = append(parts, r.statement(r.plan.test.test.statement))
		return strings.Join(parts, blockGap)
	}
	parts = append(parts, r.suite(depth))
	return strings.Join(parts, blockGap)
}

// suite writes the call of the suite that opens the next depth around its block.
func (r renderer) suite(depth int) string {
	c := r.plan.test.suites[depth].call
	body := r.suiteBlock(c)
	head := r.text(textedit.Span{Start: int(c.statement.StartByte()), End: int(body.StartByte()) + 1})
	foot := r.text(textedit.Span{Start: int(body.EndByte()) - 1, End: int(c.statement.EndByte())})
	inner := textedit.Indent(r.block(depth+1), blockIndent)
	return textedit.Tidy(r.dedented(c.statement, head) + lineBreak + inner + lineBreak + foot)
}

// statement writes a statement of the source at the left margin.
func (r renderer) statement(node *gotreesitter.Node) string {
	return textedit.Tidy(r.dedented(node, r.text(syntax.Span(node))))
}

func (r renderer) text(span textedit.Span) string { return textedit.Render(r.Source, span, r.edits) }

// dedented moves the text of a node that starts a line to the left margin.
func (r renderer) dedented(node *gotreesitter.Node, text string) string {
	return textedit.Dedent(text, textedit.IndentationOf(r.Source, int(node.StartByte())))
}
