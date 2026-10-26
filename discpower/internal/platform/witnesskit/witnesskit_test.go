package witnesskit_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/platform/witnesskit"
)

const (
	verifiesSP1979 = " [verifies SP-1979~1]"
	verifiesSP1980 = " [verifies SP-1980~1]"

	pythonTerminator = ':'
	goTerminator     = '{'
	shortID          = "0123456789ab"
	extraCount       = 50
	tokenPattern     = `[A-Za-z_][A-Za-z0-9_]{0,10}`
)

func TestHeader(t *testing.T) {
	for _, c := range []struct {
		name       string
		text       string
		terminator byte
		want       string
		tag        string
	}{
		{"a python function is cut at the colon outside its brackets", "def f(a: int = (1, 2)) -> dict[str, int]:\n    return {}", pythonTerminator, "def f(a: int = (1, 2)) -> dict[str, int]", verifiesSP1979},
		{"a colon in a default string does not end a python header", "def f(a: str = \":\") -> str:\n    pass", pythonTerminator, "def f(a: str = \":\") -> str", verifiesSP1979},
		{"a colon in a quoted return annotation does not end a python header", "def f() -> \"a:b\":\n    pass", pythonTerminator, "def f() -> \"a:b\"", verifiesSP1979},
		{"an escaped quote does not end the quoted text", `def f(a="x\":y"):`, pythonTerminator, `def f(a="x\":y")`, verifiesSP1979},
		{"a single-quoted colon does not end a python header", "class A(Base, tag=':'):\n    x: int", pythonTerminator, "class A(Base, tag=':')", verifiesSP1979},
		{"a nested bracket colon does not end a python header", "def f(a: dict[str, list[int]] = {1: 2}):", pythonTerminator, "def f(a: dict[str, list[int]] = {1: 2})", verifiesSP1979},
		{"a header without terminator is returned whole and trimmed", "  def f(a)  \n", pythonTerminator, "def f(a)", verifiesSP1979},
		{"an unterminated bracket returns the text whole", "def f(a:", pythonTerminator, "def f(a:", verifiesSP1979},
		{"an empty text gives an empty header", "", pythonTerminator, "", verifiesSP1979},
		{"a go method is cut at the brace outside its parameters", "func (r *T) F(x struct{ a int }) (int, error) {\n\treturn 0, nil\n}", goTerminator, "func (r *T) F(x struct{ a int }) (int, error)", verifiesSP1980},
		{"a go function with a struct result is cut after the result", "func New() *T {\n}", goTerminator, "func New() *T", verifiesSP1980},
		{"a brace in a go raw string does not end the header", "func F(tag string `x{`) {\n}", goTerminator, "func F(tag string `x{`)", verifiesSP1980},
		{"a go interface method list stays inside the header", "func F(v interface{ M() }) {", goTerminator, "func F(v interface{ M() })", verifiesSP1980},
	} {
		t.Run(c.name+c.tag, func(t *testing.T) {
			if got := witnesskit.Header(c.text, c.terminator); got != c.want {
				t.Errorf("Header() = %q, want %q", got, c.want)
			}
		})
	}
	t.Run("a python header is cut before the colon whatever the names"+verifiesSP1979, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			name := rapid.StringMatching(tokenPattern).Draw(t, "name")
			parameter := rapid.StringMatching(tokenPattern).Draw(t, "parameter")
			text := fmt.Sprintf("def %s(%s: int):\n    return 1", name, parameter)
			want := fmt.Sprintf("def %s(%s: int)", name, parameter)
			if got := witnesskit.Header(text, pythonTerminator); got != want {
				t.Fatalf("Header() = %q, want %q", got, want)
			}
		})
	})
}

func TestRenameAndDeclaration(t *testing.T) {
	code := "def test_fee():\n    assert fee(1) == 2\n\ndef test_fee():\n    pass\n"
	t.Run("only the first declaration of the name is renamed"+verifiesSP1979, func(t *testing.T) {
		renamed, ok := witnesskit.Rename(code, witnesskit.Declaration("def", "test_fee"), witnesskit.RenamedPrefix+shortID)
		want := "def " + witnesskit.RenamedPrefix + shortID + "():\n    assert fee(1) == 2\n\ndef test_fee():\n    pass\n"
		if !ok || renamed != want {
			t.Errorf("got %q, %v", renamed, ok)
		}
	})
	t.Run("a call of the function is not a declaration"+verifiesSP1979, func(t *testing.T) {
		callOnly := "def helper():\n    test_fee()\n"
		renamed, ok := witnesskit.Rename(callOnly, witnesskit.Declaration("def", "test_fee"), "other")
		if ok || renamed != callOnly {
			t.Errorf("got %q, %v", renamed, ok)
		}
	})
	t.Run("an absent declaration is reported and the code is unchanged"+verifiesSP1979, func(t *testing.T) {
		renamed, ok := witnesskit.Rename(code, witnesskit.Declaration("def", "test_other"), "other")
		if ok || renamed != code {
			t.Errorf("got %q, %v", renamed, ok)
		}
	})
	t.Run("a name that only starts like the declared one is not renamed"+verifiesSP1979, func(t *testing.T) {
		if _, ok := witnesskit.Rename("def test_fee_extra():\n    pass\n", witnesskit.Declaration("def", "test_fee"), "other"); ok {
			t.Error("a longer name was renamed")
		}
	})
	t.Run("an indented declaration is renamed and the indentation kept"+verifiesSP1979, func(t *testing.T) {
		renamed, ok := witnesskit.Rename("class T:\n    def test_fee(self):\n        pass\n", witnesskit.Declaration("def", "test_fee"), "renamed")
		if !ok || renamed != "class T:\n    def renamed(self):\n        pass\n" {
			t.Errorf("got %q, %v", renamed, ok)
		}
	})
	t.Run("a name with regular expression characters is matched literally"+verifiesSP1979, func(t *testing.T) {
		if _, ok := witnesskit.Rename("def a.b():\n", witnesskit.Declaration("def", "a.b"), "x"); !ok {
			t.Error("the literal name was not matched")
		}
		if _, ok := witnesskit.Rename("def aXb():\n", witnesskit.Declaration("def", "a.b"), "x"); ok {
			t.Error("the dot matched another character")
		}
	})
	t.Run("a go declaration is renamed after its keyword"+verifiesSP1980, func(t *testing.T) {
		renamed, ok := witnesskit.Rename("func TestFee(t *testing.T) {\n}\n", witnesskit.Declaration("func", "TestFee"), "TestRenamed")
		if !ok || renamed != "func TestRenamed(t *testing.T) {\n}\n" {
			t.Errorf("got %q, %v", renamed, ok)
		}
	})
	t.Run("a renamed name joins the language prefix, the witness prefix and the short id"+verifiesSP1979, func(t *testing.T) {
		if got := witnesskit.RenamedName("Test", shortID); got != "Test"+witnesskit.RenamedPrefix+shortID {
			t.Errorf("got %q", got)
		}
		if got := witnesskit.RenamedName("", shortID); got != witnesskit.RenamedPrefix+shortID {
			t.Errorf("got %q", got)
		}
	})
}

func TestMentions(t *testing.T) {
	mentioned := witnesskit.Mentions("The `price` function and Order.total() apply; price_2 is not price.")
	for _, c := range []struct {
		name string
		want bool
	}{
		{"price", true}, {"Order", true}, {"total", true}, {"price_2", true}, {"cost", false}, {"pric", false}, {"", false},
	} {
		t.Run(fmt.Sprintf("%q mentioned is %v", c.name, c.want)+verifiesSP1979, func(t *testing.T) {
			if got := mentioned(c.name); got != c.want {
				t.Errorf("got %v", got)
			}
		})
	}
	t.Run("every identifier of a text is mentioned by it"+verifiesSP1979, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			words := rapid.SliceOfN(rapid.StringMatching(tokenPattern), 1, 8).Draw(t, "words")
			mentions := witnesskit.Mentions(strings.Join(words, " + "))
			for _, word := range words {
				if !mentions(word) {
					t.Fatalf("%q is not mentioned", word)
				}
			}
		})
	})
}

func entries(count int) []witnesskit.Entry {
	out := make([]witnesskit.Entry, count)
	for index := range out {
		out[index] = witnesskit.Entry{Names: []string{fmt.Sprintf("f%d", index), "Owner"}, Signature: fmt.Sprintf("def f%d()", index)}
	}
	return out
}

func TestChoose(t *testing.T) {
	list := []witnesskit.Entry{
		{Names: []string{"price", "Order"}, Signature: "def price(self)"},
		{Names: []string{"refund"}, Signature: "def refund()"},
		{Names: []string{"total", "Order"}, Signature: "def total(self)"},
	}
	t.Run("the entries a spec mentions by their name are kept in order"+verifiesSP1979, func(t *testing.T) {
		got := witnesskit.Choose(list, witnesskit.Mentions("total and price"))
		if !slices.Equal(got, []string{"def price(self)", "def total(self)"}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("the name of an owner keeps all its entries"+verifiesSP1979, func(t *testing.T) {
		got := witnesskit.Choose(list, witnesskit.Mentions("the Order class"))
		if !slices.Equal(got, []string{"def price(self)", "def total(self)"}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("a spec mentioning none keeps every entry"+verifiesSP1979, func(t *testing.T) {
		got := witnesskit.Choose(list, witnesskit.Mentions("nothing relevant"))
		if !slices.Equal(got, []string{"def price(self)", "def refund()", "def total(self)"}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("the fall back is capped at the maximum"+verifiesSP1979, func(t *testing.T) {
		got := witnesskit.Choose(entries(witnesskit.MaxInterfaceEntries+extraCount), witnesskit.Mentions(""))
		if len(got) != witnesskit.MaxInterfaceEntries || got[0] != "def f0()" {
			t.Errorf("got %d entries starting %q", len(got), got[0])
		}
	})
	t.Run("a mentioned set is not capped"+verifiesSP1979, func(t *testing.T) {
		got := witnesskit.Choose(entries(witnesskit.MaxInterfaceEntries+extraCount), witnesskit.Mentions("Owner"))
		if len(got) != witnesskit.MaxInterfaceEntries+extraCount {
			t.Errorf("got %d entries", len(got))
		}
	})
	t.Run("no entry gives none"+verifiesSP1979, func(t *testing.T) {
		if got := witnesskit.Choose(nil, witnesskit.Mentions("anything")); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})
	t.Run("a choice is never larger than the cap unless the spec mentions the entries"+verifiesSP1979, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(0, 2*witnesskit.MaxInterfaceEntries).Draw(t, "count")
			got := witnesskit.Choose(entries(count), func(string) bool { return false })
			if want := min(count, witnesskit.MaxInterfaceEntries); len(got) != want {
				t.Fatalf("%d entries chosen of %d, want %d", len(got), count, want)
			}
		})
	})
}

func TestTestFiles(t *testing.T) {
	layout := ecosystem.Layout{
		ExcludedDirs:   []string{"build"},
		DependencyDirs: []string{"node_modules", ".venv"},
		TestFileGlobs:  []string{"test_*.py", "*_test.py"},
	}
	project := fstest.MapFS{
		"tests/test_b.py":                {},
		"tests/test_a.py":                {},
		"pkg/sub/fee_test.py":            {},
		"src/fee.py":                     {},
		"conftest.py":                    {},
		"README.md":                      {},
		".git/test_git.py":               {},
		".musthold/test_state.py":        {},
		"build/test_built.py":            {},
		"node_modules/pkg/test_dep.py":   {},
		".venv/lib/test_venv.py":         {},
		"pkg/build/test_nested_build.py": {},
	}
	t.Run("the test files are listed sorted outside the skipped directories"+verifiesSP1979, func(t *testing.T) {
		got, err := witnesskit.TestFiles(project, layout, nil)
		want := []string{"pkg/sub/fee_test.py", "tests/test_a.py", "tests/test_b.py"}
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("got %v, %v, want %v", got, err, want)
		}
	})
	t.Run("a file the extra predicate accepts is listed too"+verifiesSP1979, func(t *testing.T) {
		got, err := witnesskit.TestFiles(project, layout, func(file string) bool { return file == "conftest.py" })
		want := []string{"conftest.py", "pkg/sub/fee_test.py", "tests/test_a.py", "tests/test_b.py"}
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("got %v, %v, want %v", got, err, want)
		}
	})
	t.Run("the extra predicate does not bring back a skipped directory"+verifiesSP1979, func(t *testing.T) {
		got, err := witnesskit.TestFiles(project, layout, func(string) bool { return true })
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range got {
			for _, skipped := range []string{".git/", ".musthold/", "build/", "node_modules/", ".venv/"} {
				if strings.HasPrefix(file, skipped) || strings.Contains(file, "/"+skipped) {
					t.Errorf("%s is listed", file)
				}
			}
		}
	})
	t.Run("a project without test file lists none"+verifiesSP1979, func(t *testing.T) {
		got, err := witnesskit.TestFiles(fstest.MapFS{"src/a.py": {}}, layout, nil)
		if err != nil || len(got) != 0 {
			t.Errorf("got %v, %v", got, err)
		}
	})
	t.Run("a go layout lists its test files"+verifiesSP1980, func(t *testing.T) {
		goLayout := ecosystem.Layout{DependencyDirs: []string{"vendor"}, TestFileGlobs: []string{"*_test.go"}}
		files := fstest.MapFS{"a/a_test.go": {}, "a/a.go": {}, "vendor/x/x_test.go": {}}
		got, err := witnesskit.TestFiles(files, goLayout, nil)
		if err != nil || !slices.Equal(got, []string{"a/a_test.go"}) {
			t.Errorf("got %v, %v", got, err)
		}
	})
}

func TestLineOf(t *testing.T) {
	text := "first\nsecond\n\nfourth"
	for _, c := range []struct {
		offset int
		want   int
	}{
		{0, 1}, {len("first"), 1}, {len("first\n"), 2}, {len("first\nsecond\n"), 3}, {len("first\nsecond\n\n"), 4}, {len(text), 4},
	} {
		t.Run(fmt.Sprintf("offset %d is on line %d", c.offset, c.want)+verifiesSP1979, func(t *testing.T) {
			if got := witnesskit.LineOf(text, c.offset); got != c.want {
				t.Errorf("got %d", got)
			}
		})
	}
	t.Run("the line of an offset is one plus the newlines before it"+verifiesSP1979, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			body := rapid.StringMatching(`[a-z\n]{0,40}`).Draw(t, "text")
			offset := rapid.IntRange(0, len(body)).Draw(t, "offset")
			if got, want := witnesskit.LineOf(body, offset), 1+strings.Count(body[:offset], "\n"); got != want {
				t.Fatalf("got %d, want %d", got, want)
			}
		})
	})
}
