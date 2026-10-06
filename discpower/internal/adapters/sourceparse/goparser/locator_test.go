package goparser_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
)

const source = `package ledger

import "fmt"

type Book struct{ total int }

type Pair[K comparable, V any] struct{}

// Add records an entry.
func (b *Book) Add(amount int) {
	b.total += amount
}

func (b Book) Total() int { return b.total }

func (p *Pair[K, V]) Key() K {
	var zero K
	return zero
}

func Format(b Book) string {
	return fmt.Sprint(b.total)
}
`

func TestLocate(t *testing.T) {
	definitions, err := goparser.NewLocator().Locate("ledger/book.go", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		qualname string
		kind     ecosystem.DefinitionKind
		prefix   string
	}{
		{"(*Book).Add", ecosystem.KindMethod, "func (b *Book) Add"},
		{"Book.Total", ecosystem.KindMethod, "func (b Book) Total"},
		{"(*Pair).Key", ecosystem.KindMethod, "func (p *Pair[K, V]) Key"},
		{"ledger.Format", ecosystem.KindFunction, "func Format"},
	}

	t.Run("functions and methods are named and spanned from the func keyword [verifies SP-72~1]", func(t *testing.T) {
		if len(definitions) != len(want) {
			t.Fatalf("definitions = %+v", definitions)
		}
		for i, w := range want {
			got := definitions[i]
			text, err := got.Span.Slice([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			if got.Qualname != w.qualname || got.Kind != w.kind || !strings.HasPrefix(string(text), w.prefix) || !strings.HasSuffix(string(text), "}") {
				t.Errorf("definition %d = %+v (%q), want %+v", i, got, text, w)
			}
		}
	})

	t.Run("lines are 1-based and cover the declaration [verifies SP-72~1]", func(t *testing.T) {
		add := definitions[0]
		lines := strings.Split(source, "\n")
		if !strings.HasPrefix(lines[add.StartLine-1], "func (b *Book) Add") || lines[add.EndLine-1] != "}" {
			t.Fatalf("lines %d-%d", add.StartLine, add.EndLine)
		}
	})

	t.Run("a source that does not parse is refused [verifies SP-72~1]", func(t *testing.T) {
		if _, err := goparser.NewLocator().Locate("broken.go", []byte("package p\nfunc F( {")); err == nil {
			t.Fatal("expected a parse error")
		}
	})

	t.Run("a function is spliced through the domain splicer [verifies SP-72~1]", func(t *testing.T) {
		splicer := mutation.Splicer{Locator: goparser.NewLocator()}
		edit := mutation.Edit{Qualname: "(*Book).Add", NewSource: "func (b *Book) Add(amount int) {\n\tb.total -= amount\n}"}
		got, err := splicer.Splice("ledger/book.go", []byte(source), edit)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), "b.total -= amount") || !strings.Contains(string(got), "// Add records an entry.") {
			t.Fatalf("spliced = %s", got)
		}
	})
}
