package javascript

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
var injectedName = regexp.MustCompile(`\b[a-z]+[A-Z0-9]{2}\b`)

const (
	placeholder      = "N"
	seedsCompared    = 40
	minDistinctNames = 20
	minPositions     = 4
)

func sentinelSources(t testing.TB, source string, kind operator.SentinelKind) []string {
	t.Helper()
	return sentinelSourcesAt(t, typescriptPath, source, kind)
}

// sentinelSourcesAt lists the new sources of a kind, the injected identifiers
// replaced by a placeholder.
func sentinelSourcesAt(t testing.TB, path, source string, kind operator.SentinelKind) []string {
	t.Helper()
	candidates, err := New().SentinelCandidates(fileAt(path, source), operator.LocalSeed)
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
	t.Run("a dead assignment is offered before each statement, indented like it, and is read by void [verifies SP-1001~1]", func(t *testing.T) {
		source := "class A {\n  m(x) {\n    const y = x + 1;\n    return y;\n  }\n}\n"
		got := sentinelSources(t, source, operator.SentinelDeadAssignment)
		want := []string{
			"m(x) {\n    const N = 0; void N;\n    const y = x + 1;\n    return y;\n  }",
			"m(x) {\n    const y = x + 1;\n    const N = 0; void N;\n    return y;\n  }",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("no dead assignment goes before a directive or a call of super [verifies SP-1001~1]", func(t *testing.T) {
		source := "class A extends B {\n  constructor(x) {\n    super(x);\n    this.x = x;\n  }\n}\nfunction f() {\n  'use strict';\n  return 1;\n}\n"
		got := sentinelSources(t, source, operator.SentinelDeadAssignment)
		want := []string{
			"constructor(x) {\n    super(x);\n    const N = 0; void N;\n    this.x = x;\n  }",
			"function f() {\n  'use strict';\n  const N = 0; void N;\n  return 1;\n}",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a one-line body and an empty body get no dead assignment [verifies SP-1001~1]", func(t *testing.T) {
		if got := sentinelSources(t, "function f() { return 1; }\n\nfunction g() {}\n", operator.SentinelDeadAssignment); len(got) != 0 {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a local is renamed everywhere except property names [verifies SP-1001~1]", func(t *testing.T) {
		source := "function f(a) {\n  const rate = a.rate;\n  return g({ rate: rate, other: rate + 1 });\n}\n"
		got := sentinelSources(t, source, operator.SentinelAlphaRename)
		want := []string{"function f(a) {\n  const N = a.rate;\n  return g({ rate: N, other: N + 1 });\n}"}
		if !slices.Equal(got, want) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a let and a var local are renamed with the occurrences of a loop [verifies SP-1001~1]", func(t *testing.T) {
		source := "function f(xs) {\n  var total = 0;\n  for (let i = 0; i < xs.length; i++) {\n    total += xs[i];\n  }\n  return total;\n}\n"
		got := sentinelSources(t, source, operator.SentinelAlphaRename)
		want := []string{
			"function f(xs) {\n  var N = 0;\n  for (let i = 0; i < xs.length; i++) {\n    N += xs[i];\n  }\n  return N;\n}",
			"function f(xs) {\n  var total = 0;\n  for (let N = 0; N < xs.length; N++) {\n    total += xs[N];\n  }\n  return total;\n}",
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("parameters, patterns, loops, catch clauses, shorthand properties and closure names are never renamed [verifies SP-1001~1]", func(t *testing.T) {
		sources := []string{
			"function f(a) {\n  var a = 1;\n  return a;\n}\n",
			"function f(o) {\n  const { x } = o;\n  return x;\n}\n",
			"function f(o) {\n  const { x: y } = o;\n  return y;\n}\n",
			"function f(o) {\n  const [y] = o;\n  return y;\n}\n",
			"function f(xs) {\n  for (const x of xs) {\n    run(x);\n  }\n}\n",
			"function f() {\n  try {\n    run();\n  } catch (e) {\n    return e;\n  }\n}\n",
			"function f() {\n  const x = 1;\n  return { x };\n}\n",
			"const total = 5;\nfunction f() {\n  const total = 1;\n  return total;\n}\n",
			"function f(c) {\n  {\n    let t = 1;\n    run(t);\n  }\n  {\n    const t = 2;\n    run(t);\n  }\n}\n",
			"function f() {\n  const f = 1;\n  return f;\n}\n",
		}
		for _, source := range sources {
			if got := sentinelSources(t, source, operator.SentinelAlphaRename); len(got) != 0 {
				t.Errorf("renamed in %q: %q", source, got)
			}
		}
	})

	t.Run("a name mentioned by a nested function is not renamed while the others are [verifies SP-1001~1]", func(t *testing.T) {
		source := "function f() {\n  const n = 1;\n  const inner = () => n;\n  return inner;\n}\n"
		got := sentinelSources(t, source, operator.SentinelAlphaRename)
		want := []string{"function f() {\n  const n = 1;\n  const N = () => n;\n  return N;\n}"}
		if !slices.Equal(got, want) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a function that mentions eval, Function or arguments, or uses with, is skipped by every function sentinel [verifies SP-1001~1]", func(t *testing.T) {
		sources := []string{
			"function f(a) {\n  const value = eval(a);\n  return value;\n}\n",
			"function f(a) {\n  const value = new Function(a);\n  return value;\n}\n",
			"function f() {\n  const value = arguments[0];\n  return value;\n}\n",
			"function f(a) {\n  with (a) {\n    run();\n  }\n  const value = 1;\n  return value;\n}\n",
		}
		for _, source := range sources {
			for _, kind := range []operator.SentinelKind{operator.SentinelDeadAssignment, operator.SentinelAlphaRename} {
				if got := sentinelSources(t, source, kind); len(got) != 0 {
					t.Errorf("%s sentinel proposed in %q: %q", kind, source, got)
				}
			}
		}
	})

	t.Run("the unused constant is exported in a module and plain in a script, with a fresh name [verifies SP-1001~1]", func(t *testing.T) {
		cases := []struct {
			path, source, prefix string
		}{
			{"m.ts", "export const _SP_UNUSED = 1;\n", "export const "},
			{"m.ts", "import x from 'x';\nconst _SP_UNUSED = x;\n", "export const "},
			{"m.js", "const _SP_UNUSED = 1;\n", "const "},
		}
		for _, tt := range cases {
			candidates, err := New().SentinelCandidates(fileAt(tt.path, tt.source), operator.LocalSeed)
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
				constant.Edit.NewSource != tt.prefix+constant.Edit.Qualname+" = 0;" || strings.Contains(tt.source, constant.Edit.Qualname) {
				t.Fatalf("%q: constant = %+v", tt.source, constant)
			}
		}
	})

	t.Run("every function sentinel parses and records the original text of its span [verifies SP-1001~1]", func(t *testing.T) {
		source := "function f(a) {\n  const value = a + 1;\n  return value;\n}\n"
		candidates, err := New().SentinelCandidates(file(source), operator.LocalSeed)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range candidates {
			if c.Kind == operator.SentinelUnusedConstant {
				continue
			}
			if !c.Replaces.Within(len(source)) || c.Original != source[c.Replaces.Start:c.Replaces.End] {
				t.Fatalf("original text does not match its span: %+v", c)
			}
			if _, err := New().Normalize(typescriptPath, c.Edit.Target, c.Edit.NewSource); err != nil {
				t.Fatalf("%s does not parse: %v", c.Kind, err)
			}
		}
	})

	t.Run("injected identifiers and positions change with the seed and carry no fixed marker [verifies SP-1001~1]", func(t *testing.T) {
		content, err := os.ReadFile(goldenSourcePath)
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		positions := map[ecosystem.Span]bool{}
		for seed := operator.Seed(1); seed <= seedsCompared; seed++ {
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
		if len(names) < minDistinctNames || len(positions) < minPositions {
			t.Fatalf("%d names and %d positions over %d seeds", len(names), len(positions), seedsCompared)
		}
	})
}
