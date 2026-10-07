package textedit_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/alignment/internal/platform/textedit"
)

func TestRender(t *testing.T) {
	const source = "abc def ghi"
	whole := textedit.Span{Start: 0, End: len(source)}

	t.Run("the edits inside the span are applied and the others ignored [verifies SP-1215~1]", func(t *testing.T) {
		got := textedit.Render([]byte(source), textedit.Span{Start: 4, End: 7}, []textedit.Edit{
			{Span: textedit.Span{Start: 4, End: 5}, Replacement: "D"},
			{Span: textedit.Span{Start: 0, End: 3}, Replacement: "X"},
		})
		if got != "Def" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("an edit nested in a longer one is dropped and an overlapping one is cut [verifies SP-1215~1]", func(t *testing.T) {
		nested := textedit.Render([]byte(source), whole, []textedit.Edit{
			{Span: textedit.Span{Start: 0, End: 7}, Replacement: "_"},
			{Span: textedit.Span{Start: 2, End: 3}, Replacement: "!"},
		})
		if nested != "_ ghi" {
			t.Fatalf("nested %q", nested)
		}
		overlapping := textedit.Render([]byte(source), whole, []textedit.Edit{
			{Span: textedit.Span{Start: 0, End: 5}, Replacement: "_"},
			{Span: textedit.Span{Start: 3, End: 7}, Replacement: "-"},
		})
		if overlapping != "_- ghi" {
			t.Fatalf("overlapping %q", overlapping)
		}
	})

	t.Run("without edits the text of the span is returned as it is [verifies SP-1215~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			text := rapid.String().Draw(t, "text")
			if got := textedit.Render([]byte(text), textedit.Span{Start: 0, End: len(text)}, nil); got != text {
				t.Fatalf("got %q, want %q", got, text)
			}
		})
	})
}

func TestRemoveText(t *testing.T) {
	const source = "a\n    // note\nb // tail\n"

	t.Run("a comment alone on its line takes the line with it [verifies SP-1215~1]", func(t *testing.T) {
		start := 6
		edit := textedit.RemoveText([]byte(source), textedit.Span{Start: start, End: start + len("// note")})
		got := textedit.Render([]byte(source), textedit.Span{Start: 0, End: len(source)}, []textedit.Edit{edit})
		if got != "a\nb // tail\n" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a comment after code leaves the code and its line [verifies SP-1215~1]", func(t *testing.T) {
		start := len("a\n    // note\nb ")
		edit := textedit.RemoveText([]byte(source), textedit.Span{Start: start, End: start + len("// tail")})
		got := textedit.Render([]byte(source), textedit.Span{Start: 0, End: len(source)}, []textedit.Edit{edit})
		if got != "a\n    // note\nb \n" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestTidyAndLayout(t *testing.T) {
	t.Run("trailing blanks go, runs of blank lines collapse and both ends are trimmed [verifies SP-1215~1]", func(t *testing.T) {
		got := textedit.Tidy("\n\nfirst  \n\n\n\nsecond\t\n\n")
		if got != "first\n\nsecond" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("blocks are joined one blank line apart and empty ones are skipped [verifies SP-1215~1]", func(t *testing.T) {
		if got := textedit.Join([]string{"a", "", "b"}); got != "a\n\nb\n" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("dedenting and indenting are inverses on indented text [verifies SP-1215~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			body := rapid.StringMatching(`([a-z]+\n){1,5}`).Draw(t, "body")
			indented := textedit.Indent(body, "    ")
			if got := textedit.DedentAll(indented, 4); got != body {
				t.Fatalf("got %q, want %q", got, body)
			}
		})
	})

	t.Run("a line start is found only when blanks alone precede the offset [verifies SP-1215~1]", func(t *testing.T) {
		source := []byte("x\n    code; more")
		if got := textedit.LineStart(source, 6); got != 2 {
			t.Errorf("line start %d", got)
		}
		if got := textedit.LineStart(source, 12); got != 12 {
			t.Errorf("mid-line %d", got)
		}
	})
}
