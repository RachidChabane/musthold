package treesitter

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

type expectedDefinition struct {
	qualname string
	kind     ecosystem.DefinitionKind
	prefix   string
}

const (
	pythonSource = "class A:\n    def m(self):\n        def inner():\n            pass\n        return 1\n\ndef f(x):\n    return x\n"

	javaSource = "class A {\n  A() {}\n  int m(int x) { return x; }\n  static class B {\n    void n() {}\n  }\n}\n"

	kotlinSource = "class A {\n  fun m(x: Int): Int { return x }\n}\nfun top() {}\nobject O {\n  fun q() {}\n}\n"

	typescriptSource = "class A {\n  m(x: number): number { return x }\n}\nfunction f(): void {}\n"

	tsxSource = "class A {\n  render() { return <div/> }\n}\nfunction View() { return <span/> }\n"

	javascriptSource = "class A {\n  m(x) { return x }\n}\nfunction f() {}\n"
)

func TestLocateFindsDefinitionsOfEveryLanguage(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		source   string
		expected []expectedDefinition
	}{
		{"python [verifies SP-41~1]", "pkg/mod.py", pythonSource, []expectedDefinition{
			{"A.m", ecosystem.KindMethod, "def m(self):"},
			{"A.m.inner", ecosystem.KindFunction, "def inner():"},
			{"f", ecosystem.KindFunction, "def f(x):"},
		}},
		{"java [verifies SP-41~1]", "src/A.java", javaSource, []expectedDefinition{
			{"A.A", ecosystem.KindMethod, "A() {}"},
			{"A.m", ecosystem.KindMethod, "int m(int x)"},
			{"A.B.n", ecosystem.KindMethod, "void n() {}"},
		}},
		{"kotlin [verifies SP-41~1]", "src/A.kt", kotlinSource, []expectedDefinition{
			{"A.m", ecosystem.KindMethod, "fun m(x: Int)"},
			{"top", ecosystem.KindFunction, "fun top()"},
			{"O.q", ecosystem.KindMethod, "fun q()"},
		}},
		{"kotlin script [verifies SP-41~1]", "build.gradle.kts", kotlinSource, []expectedDefinition{
			{"A.m", ecosystem.KindMethod, "fun m(x: Int)"},
			{"top", ecosystem.KindFunction, "fun top()"},
			{"O.q", ecosystem.KindMethod, "fun q()"},
		}},
		{"typescript [verifies SP-41~1]", "src/a.ts", typescriptSource, []expectedDefinition{
			{"A.m", ecosystem.KindMethod, "m(x: number)"},
			{"f", ecosystem.KindFunction, "function f()"},
		}},
		{"tsx [verifies SP-41~1]", "src/a.tsx", tsxSource, []expectedDefinition{
			{"A.render", ecosystem.KindMethod, "render()"},
			{"View", ecosystem.KindFunction, "function View()"},
		}},
		{"javascript [verifies SP-41~1]", "src/a.js", javascriptSource, []expectedDefinition{
			{"A.m", ecosystem.KindMethod, "m(x)"},
			{"f", ecosystem.KindFunction, "function f()"},
		}},
		{"javascript module [verifies SP-41~1]", "src/a.mjs", javascriptSource, []expectedDefinition{
			{"A.m", ecosystem.KindMethod, "m(x)"},
			{"f", ecosystem.KindFunction, "function f()"},
		}},
		{"javascript commonjs [verifies SP-41~1]", "src/a.cjs", javascriptSource, []expectedDefinition{
			{"A.m", ecosystem.KindMethod, "m(x)"},
			{"f", ecosystem.KindFunction, "function f()"},
		}},
		{"javascript jsx [verifies SP-41~1]", "src/a.jsx", javascriptSource, []expectedDefinition{
			{"A.m", ecosystem.KindMethod, "m(x)"},
			{"f", ecosystem.KindFunction, "function f()"},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			definitions, err := NewLocator().Locate(test.path, source)
			if err != nil {
				t.Fatalf("Locate: %v", err)
			}
			if len(definitions) != len(test.expected) {
				t.Fatalf("got %d definitions %+v, want %d", len(definitions), definitions, len(test.expected))
			}
			for i, want := range test.expected {
				got := definitions[i]
				if got.Qualname != want.qualname || got.Kind != want.kind {
					t.Errorf("definition %d = %s/%s, want %s/%s", i, got.Qualname, got.Kind, want.qualname, want.kind)
				}
				text, err := got.Span.Slice(source)
				if err != nil {
					t.Fatalf("definition %d span: %v", i, err)
				}
				if !strings.HasPrefix(string(text), want.prefix) {
					t.Errorf("definition %d text %q does not start with %q", i, text, want.prefix)
				}
			}
		})
	}
}

func TestLocateNamesNestedDefinitionsByQualifiedPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		source   string
		expected []string
	}{
		{"python class method [verifies SP-42~1]", "a.py", "class A:\n    def m(self): pass\n", []string{"A.m"}},
		{"python nested function [verifies SP-42~1]", "a.py", pythonSource, []string{"A.m", "A.m.inner", "f"}},
		{"python nested class [verifies SP-42~1]", "a.py", "class A:\n    class B:\n        def m(self): pass\n", []string{"A.B.m"}},
		{"python top level [verifies SP-42~1]", "a.py", "def f(): pass\n", []string{"f"}},
		{"java nested class [verifies SP-42~1]", "A.java", javaSource, []string{"A.A", "A.m", "A.B.n"}},
		{"java interface [verifies SP-42~1]", "I.java", "interface I {\n  default void d() {}\n}\n", []string{"I.d"}},
		{"kotlin object [verifies SP-42~1]", "A.kt", kotlinSource, []string{"A.m", "top", "O.q"}},
		{"typescript class [verifies SP-42~1]", "a.ts", typescriptSource, []string{"A.m", "f"}},
		{"javascript nested function [verifies SP-42~1]", "a.js", "function outer() {\n  function inner() {}\n}\n", []string{"outer", "outer.inner"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definitions, err := NewLocator().Locate(test.path, []byte(test.source))
			if err != nil {
				t.Fatalf("Locate: %v", err)
			}
			var got []string
			for _, definition := range definitions {
				got = append(got, definition.Qualname)
			}
			if strings.Join(got, ",") != strings.Join(test.expected, ",") {
				t.Fatalf("qualnames = %v, want %v", got, test.expected)
			}
		})
	}
}

func TestLocateKeepsDefinitionsOfSourceWithSyntaxErrors(t *testing.T) {
	source := []byte("def good(x):\n    return x\n\ndef broken(:\n")
	definitions, err := NewLocator().Locate("broken.py", source)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if len(definitions) == 0 || definitions[0].Qualname != "good" {
		t.Fatalf("definitions = %+v, want good first", definitions)
	}
}

func TestLocateRefusesUnsupportedExtension(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"ruby [verifies SP-44~1]", "app.rb"},
		{"no extension [verifies SP-44~1]", "Makefile"},
		{"upper case extension [verifies SP-44~1]", "A.PY"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewLocator().Locate(test.path, []byte("x"))
			if !errors.Is(err, ErrUnsupportedLanguage) {
				t.Fatalf("err = %v, want ErrUnsupportedLanguage", err)
			}
		})
	}
}

func TestLocatePythonSpansStayInsideSourceAndAreOrdered(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		source := []byte(pythonProgram().Draw(t, "program"))
		definitions, err := NewLocator().Locate("gen.py", source)
		if err != nil {
			t.Fatalf("Locate: %v", err)
		}
		previousStart := -1
		for _, definition := range definitions {
			if !definition.Span.Within(len(source)) {
				t.Fatalf("span %+v outside a source of %d bytes", definition.Span, len(source))
			}
			if definition.Span.Start < previousStart {
				t.Fatalf("definitions not ordered by start: %+v", definitions)
			}
			previousStart = definition.Span.Start
		}
	})
}

func pythonProgram() *rapid.Generator[string] {
	name := rapid.StringMatching(`[a-z][a-z0-9_]{0,6}`)
	line := rapid.Custom(func(t *rapid.T) string {
		indent := strings.Repeat("    ", rapid.IntRange(0, 2).Draw(t, "indent"))
		keyword := rapid.SampledFrom([]string{"def %s(x):", "class %s:", "%s = 1"}).Draw(t, "keyword")
		return indent + fmt.Sprintf(keyword, name.Draw(t, "name")) + "\n"
	})
	return rapid.Custom(func(t *rapid.T) string {
		return strings.Join(rapid.SliceOfN(line, 0, 12).Draw(t, "lines"), "")
	})
}
