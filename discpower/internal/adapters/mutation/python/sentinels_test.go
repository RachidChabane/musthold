package python

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

// injectedName matches the identifiers sentinels inject: a word and a suffix.
var injectedName = regexp.MustCompile(`\b[a-z]+_[a-z0-9]{2}\b`)

const placeholder = "N"

func sentinelSources(t testing.TB, source string, kind operator.SentinelKind) []string {
	t.Helper()
	return sentinelSourcesWith(t, source, kind, operator.LocalSeed)
}

// sentinelSourcesWith lists the new sources of a kind, the injected identifiers
// replaced by a placeholder.
func sentinelSourcesWith(t testing.TB, source string, kind operator.SentinelKind, seed operator.Seed) []string {
	t.Helper()
	candidates, err := New().SentinelCandidates(file(source), seed)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range candidates {
		if c.Kind == kind {
			out = append(out, injectedName.ReplaceAllString(c.Edit.NewSource, placeholder))
		}
	}
	return out
}

func TestSentinels(t *testing.T) {
	t.Run("a dead assignment is offered before each statement after the docstring, indented like it [verifies SP-92~1]", func(t *testing.T) {
		source := "class A:\n    def m(self, x):\n        \"\"\"Doc.\"\"\"\n        y = x + 1\n        return y\n"
		got := sentinelSources(t, source, operator.SentinelDeadAssignment)
		want := []string{
			"def m(self, x):\n        \"\"\"Doc.\"\"\"\n        N = 0\n        y = x + 1\n        return y",
			"def m(self, x):\n        \"\"\"Doc.\"\"\"\n        y = x + 1\n        N = 0\n        return y",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a one-line body and a docstring-only body get no dead assignment [verifies SP-92~1]", func(t *testing.T) {
		if got := sentinelSources(t, "def f(): return 1\n\ndef g():\n    \"\"\"Only.\"\"\"\n", operator.SentinelDeadAssignment); len(got) != 0 {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a local is renamed everywhere except attribute names and keyword names [verifies SP-92~1]", func(t *testing.T) {
		source := "def f(a):\n    rate = a.rate\n    return g(rate=rate, other=rate + 1)\n"
		got := sentinelSources(t, source, operator.SentinelAlphaRename)
		want := []string{"def f(a):\n    N = a.rate\n    return g(rate=N, other=N + 1)"}
		if !slices.Equal(got, want) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("parameters, closure names, comprehension targets and scope declarations are never renamed [verifies SP-92~1]", func(t *testing.T) {
		sources := []string{
			"def f(a):\n    a = a + 1\n    return a\n",
			"def f():\n    n = 1\n    def inner():\n        return n\n    return inner\n",
			"def f(xs):\n    x = 1\n    return [x for x in xs]\n",
			"def f():\n    global total\n    total = 1\n",
			"def f():\n    count = 0\n    def bump():\n        nonlocal count\n        count += 1\n    return bump\n",
		}
		for _, source := range sources {
			if got := sentinelSources(t, source, operator.SentinelAlphaRename); len(got) != 0 {
				t.Errorf("renamed in %q: %q", source, got)
			}
		}
	})

	t.Run("a function that observes locals or runs text is skipped by every function sentinel [verifies SP-92~1]", func(t *testing.T) {
		for _, name := range introspectionNames {
			source := "def f(a):\n    value = " + name + "(a)\n    return value\n"
			for _, kind := range []operator.SentinelKind{operator.SentinelDeadAssignment, operator.SentinelAlphaRename} {
				if got := sentinelSources(t, source, kind); len(got) != 0 {
					t.Errorf("%s: %s sentinel proposed: %q", name, kind, got)
				}
			}
		}
		source := "import inspect\n\ndef f(a):\n    value = inspect.stack()\n    return value\n"
		if got := sentinelSources(t, source, operator.SentinelDeadAssignment); len(got) != 0 {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("the unused constant is a private module addition with a fresh name [verifies SP-92~1]", func(t *testing.T) {
		candidates, err := New().SentinelCandidates(file("_SP_UNUSED = 1\n"), operator.LocalSeed)
		if err != nil {
			t.Fatal(err)
		}
		var constant *operator.SentinelCandidate
		for i, c := range candidates {
			if c.Kind == operator.SentinelUnusedConstant {
				constant = &candidates[i]
			}
		}
		if constant == nil || constant.Edit.Target != mutation.TargetModuleAddition || !strings.HasPrefix(constant.Edit.Qualname, "_") ||
			constant.Edit.NewSource != constant.Edit.Qualname+" = 0" || strings.Contains("_SP_UNUSED = 1\n", constant.Edit.Qualname) {
			t.Fatalf("constant = %+v", constant)
		}
	})

	t.Run("every function sentinel parses and records the original text of its span [verifies SP-92~1]", func(t *testing.T) {
		source := "def f(a):\n    value = a + 1\n    return value\n"
		candidates, err := New().SentinelCandidates(file(source), operator.LocalSeed)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range candidates {
			if c.Kind == operator.SentinelUnusedConstant {
				continue
			}
			if c.Replaces.Within(len(source)) == false || c.Original != source[c.Replaces.Start:c.Replaces.End] {
				t.Fatalf("original text does not match its span: %+v", c)
			}
			if _, err := New().Normalize("m.py", c.Edit.Target, c.Edit.NewSource); err != nil {
				t.Fatalf("%s does not parse: %v", c.Kind, err)
			}
		}
	})

	t.Run("injected identifiers and positions change with the seed and carry no fixed marker [verifies SP-89~1]", func(t *testing.T) {
		content, err := os.ReadFile(goldenSourcePath)
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		positions := map[ecosystem.Span]bool{}
		for seed := operator.Seed(1); seed <= 40; seed++ {
			candidates, err := New().SentinelCandidates(mutation.SourceFile{Path: goldenName, Content: content}, seed)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range candidates {
				for _, marker := range []string{"_sp_", "_SP_", "SP_UNUSED"} {
					if strings.Contains(c.Edit.NewSource, marker) {
						t.Fatalf("%s sentinel carries the marker %q:\n%s", c.Kind, marker, c.Edit.NewSource)
					}
				}
				for _, name := range injectedName.FindAllString(c.Edit.NewSource, -1) {
					names[name] = true
				}
				if c.Kind == operator.SentinelDeadAssignment {
					positions[c.Site.Span] = true
				}
			}
		}
		if len(names) < 20 || len(positions) < 4 {
			t.Fatalf("%d names and %d positions over 40 seeds", len(names), len(positions))
		}
	})
}
