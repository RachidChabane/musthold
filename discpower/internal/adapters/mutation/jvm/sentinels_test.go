package jvm

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

// injectedLocal matches the locals sentinels inject: a word and a two character
// suffix in camel case.
var injectedLocal = regexp.MustCompile(`\b[a-z]+[A-Z0-9][a-z0-9]\b`)

// injectedConstant matches the constant of the unused constant sentinel.
var injectedConstant = regexp.MustCompile(`^[A-Z]+_[A-Z0-9]{2}$`)

const placeholder = "N"

// sentinelSources lists the new sources of a kind, the injected identifiers
// replaced by a placeholder.
func sentinelSources(t testing.TB, path, source string, kind operator.SentinelKind) []string {
	t.Helper()
	var out []string
	for _, c := range sentinelsOf(t, path, source, operator.LocalSeed) {
		if c.Kind == kind {
			out = append(out, injectedLocal.ReplaceAllString(c.Edit.NewSource, placeholder))
		}
	}
	return out
}

func TestDeadAssignments(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		source string
		want   []string
	}{
		{"java local before each statement that starts a line", javaPath,
			"class T {\n  int m(int x) {\n    int y = x + 1;\n    return y;\n  }\n}\n",
			[]string{
				"int m(int x) {\n    int N = 0;\n    int y = x + 1;\n    return y;\n  }",
				"int m(int x) {\n    int y = x + 1;\n    int N = 0;\n    return y;\n  }",
			}},
		{"java explicit constructor invocation stays first", javaPath,
			"class T {\n  T(int x) {\n    super();\n    int y = x;\n  }\n}\n",
			[]string{"T(int x) {\n    super();\n    int N = 0;\n    int y = x;\n  }"}},
		{"java one-line and empty bodies get none", javaPath,
			"class T {\n  int f() { return 1; }\n  void g() {\n  }\n  abstract int h();\n}\n", nil},
		{"kotlin local before each statement that starts a line", kotlinPath,
			"fun m(x: Int): Int {\n    val y = x + 1\n    return y\n}\n",
			[]string{
				"fun m(x: Int): Int {\n    val N = 0\n    val y = x + 1\n    return y\n}",
				"fun m(x: Int): Int {\n    val y = x + 1\n    val N = 0\n    return y\n}",
			}},
		{"kotlin expression body and labeled statement get none before the label target", kotlinPath,
			"fun f(x: Int) = x + 1\nfun g(n: Int) {\n    loop@\n    for (i in 0..n) { break@loop }\n}\n",
			[]string{"fun g(n: Int) {\n    val N = 0\n    loop@\n    for (i in 0..n) { break@loop }\n}"}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" [verifies SP-1102~1]", func(t *testing.T) {
			got := sentinelSources(t, tt.path, tt.source, operator.SentinelDeadAssignment)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestRenames(t *testing.T) {
	safe := []struct {
		name   string
		path   string
		source string
		want   []string
	}{
		{"java local renamed at every use", javaPath,
			"class T {\n  int f(int a) {\n    int rate = a + 1;\n    return g(rate, rate);\n  }\n}\n",
			[]string{"int f(int a) {\n    int N = a + 1;\n    return g(N, N);\n  }"}},
		{"java for variable renamed within the loop", javaPath,
			"class T {\n  int f(int a) {\n    for (int i = 0; i < a; i++) { a += i; }\n    return a;\n  }\n}\n",
			[]string{"int f(int a) {\n    for (int N = 0; N < a; N++) { a += N; }\n    return a;\n  }"}},
		{"kotlin local renamed including its template uses", kotlinPath,
			"fun f(a: Int): String {\n    val rate = a + 1\n    return \"r=$rate ${rate}\" + rate\n}\n",
			[]string{"fun f(a: Int): String {\n    val N = a + 1\n    return \"r=$N ${N}\" + N\n}"}},
	}
	for _, tt := range safe {
		t.Run(tt.name+" [verifies SP-1102~1]", func(t *testing.T) {
			got := sentinelSources(t, tt.path, tt.source, operator.SentinelAlphaRename)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %q\nwant %q", got, tt.want)
			}
		})
	}

	unsafe := []struct {
		name   string
		path   string
		source string
	}{
		{"java declared in two blocks", javaPath, "class T {\n  int f() {\n    { int n = 1; }\n    { int n = 2; }\n    return 0;\n  }\n}\n"},
		{"java used outside its block", javaPath, "class T {\n  int n;\n  int f() {\n    { int n = 1; }\n    return n;\n  }\n}\n"},
		{"java method of the same name", javaPath, "class T {\n  int f() {\n    int count = 1;\n    return count + count();\n  }\n}\n"},
		{"java field after a dot", javaPath, "class T {\n  int f(T p) {\n    int size = 1;\n    return size + p.size + this.size;\n  }\n}\n"},
		{"java lambda parameter of the same name", javaPath, "class T {\n  int f(java.util.List<Integer> xs) {\n    xs.forEach(n -> { });\n    int n = 1;\n    return n;\n  }\n}\n"},
		{"java catch parameter of the same name", javaPath, "class T {\n  int f() {\n    try { g(); } catch (RuntimeException e) { }\n    int e = 1;\n    return e;\n  }\n}\n"},
		{"java label of the same name", javaPath, "class T {\n  int f() {\n    int outer = 1;\n    outer:\n    for (;;) { break outer; }\n    return outer;\n  }\n}\n"},
		{"java several declarators", javaPath, "class T {\n  int f() {\n    int a = 1, b = 2;\n    return a + b;\n  }\n}\n"},
		{"kotlin parameter of the same name", kotlinPath, "fun f(rate: Int): Int {\n    val rate = rate + 1\n    return rate\n}\n"},
		{"kotlin named argument", kotlinPath, "fun f(): Int {\n    val rate = 1\n    return g(rate = rate)\n}\n"},
		{"kotlin member after a dot", kotlinPath, "fun f(p: P): Int {\n    val size = 1\n    return size + p.size\n}\n"},
		{"kotlin function of the same name", kotlinPath, "fun f(): Int {\n    val count = 1\n    return count + count()\n}\n"},
		{"kotlin for variable of the same name", kotlinPath, "fun f(xs: List<Int>): Int {\n    val x = 1\n    for (x in xs) { }\n    return x\n}\n"},
		{"kotlin lambda parameter", kotlinPath, "fun f(xs: List<Int>): Int {\n    val x = 1\n    return xs.map { x -> x + 1 }.size + x\n}\n"},
		{"kotlin implicit lambda parameter", kotlinPath, "fun f(xs: List<Int>): Int {\n    val it = 1\n    return xs.map { it + 1 }.size + it\n}\n"},
		{"kotlin destructuring", kotlinPath, "fun f(p: Pair<Int, Int>): Int {\n    val (a, b) = p\n    return a + b\n}\n"},
	}
	for _, tt := range unsafe {
		t.Run(tt.name+" is never renamed [verifies SP-1102~1]", func(t *testing.T) {
			for _, c := range sentinelsOf(t, tt.path, tt.source, operator.LocalSeed) {
				if c.Kind == operator.SentinelAlphaRename {
					t.Fatalf("renamed:\n%s", c.Edit.NewSource)
				}
			}
		})
	}
}

func TestUnusedConstantIsKotlinOnly(t *testing.T) {
	t.Run("java emits no unused constant [verifies SP-1102~1]", func(t *testing.T) {
		for _, c := range sentinelsOf(t, javaPath, "class T {\n  int f(int a) {\n    int b = a;\n    return b;\n  }\n}\n", operator.LocalSeed) {
			if c.Kind == operator.SentinelUnusedConstant || c.Edit.Target == mutation.TargetModuleAddition {
				t.Fatalf("java sentinel %+v", c)
			}
		}
	})
	t.Run("kotlin appends a private constant with a fresh upper snake name [verifies SP-1102~1]", func(t *testing.T) {
		source := "const val WORD_XX = 1\nfun f() {}\n"
		var constants []operator.SentinelCandidate
		for _, c := range sentinelsOf(t, kotlinPath, source, operator.LocalSeed) {
			if c.Kind == operator.SentinelUnusedConstant {
				constants = append(constants, c)
			}
		}
		if len(constants) != 1 {
			t.Fatalf("constants %+v", constants)
		}
		c := constants[0]
		if c.Edit.Target != mutation.TargetModuleAddition || !injectedConstant.MatchString(c.Edit.Qualname) ||
			c.Edit.NewSource != "private const val "+c.Edit.Qualname+" = 0" || strings.Contains(source, c.Edit.Qualname) ||
			c.Replaces.Start != len(source) || c.Replaces.End != len(source) {
			t.Fatalf("constant %+v", c)
		}
	})
}

func TestReflectiveUnitsGetNoSentinel(t *testing.T) {
	t.Run("the lists of reflection identifiers and markers are not empty [verifies SP-1102~1]", func(t *testing.T) {
		if len(reflectionNames) == 0 || len(reflectionMarkers) == 0 {
			t.Fatal("empty reflection lists")
		}
	})
	for _, name := range reflectionNames {
		t.Run("java "+name+" [verifies SP-1102~1]", func(t *testing.T) {
			source := "class T {\n  Object f(Object o) {\n    Object value = o." + name + "();\n    return value;\n  }\n}\n"
			assertNoFunctionSentinel(t, javaPath, source)
		})
		t.Run("kotlin "+name+" [verifies SP-1102~1]", func(t *testing.T) {
			source := "fun f(o: Any): Any {\n    val value = o." + name + "()\n    return value\n}\n"
			assertNoFunctionSentinel(t, kotlinPath, source)
		})
	}
	markers := []struct{ name, path, source string }{
		{"java java.lang.reflect", javaPath, "class T {\n  Object f() {\n    java.lang.reflect.Method m = null;\n    return m;\n  }\n}\n"},
		{"kotlin ::class", kotlinPath, "fun f(o: Any): Any {\n    val k = o::class\n    return k\n}\n"},
		{"kotlin kotlin.reflect", kotlinPath, "fun f(o: Any): Any {\n    val k: kotlin.reflect.KClass<*>? = null\n    return k ?: o\n}\n"},
	}
	for _, tt := range markers {
		t.Run(tt.name+" [verifies SP-1102~1]", func(t *testing.T) {
			assertNoFunctionSentinel(t, tt.path, tt.source)
		})
	}
	t.Run("a unit next to a reflective one still gets its sentinels [verifies SP-1102~1]", func(t *testing.T) {
		source := "class T {\n  Object f(Object o) {\n    Object v = o.getClass();\n    return v;\n  }\n  int g(int a) {\n    int b = a;\n    return b;\n  }\n}\n"
		qualnames := map[string]bool{}
		for _, c := range sentinelsOf(t, javaPath, source, operator.LocalSeed) {
			qualnames[c.Edit.Qualname] = true
		}
		if qualnames["T.f"] || !qualnames["T.g"] {
			t.Fatalf("sentinel units %v", qualnames)
		}
	})
}

func assertNoFunctionSentinel(t *testing.T, path, source string) {
	t.Helper()
	for _, c := range sentinelsOf(t, path, source, operator.LocalSeed) {
		if c.Edit.Target == mutation.TargetDefinition {
			t.Fatalf("sentinel of a reflective unit:\n%s", c.Edit.NewSource)
		}
	}
}

func TestSentinelNamesAndDeterminism(t *testing.T) {
	javaSource := "class T {\n  int rate(int a) {\n    int local = a + 1;\n    int other = local * 2;\n    return other;\n  }\n}\n"
	kotlinSource := "const val PROBE_AB = 1\nfun f(a: Int): Int {\n    val local = a + 1\n    val other = local * 2\n    return other\n}\n"
	for path, source := range map[string]string{javaPath: javaSource, kotlinPath: kotlinSource} {
		t.Run(path+" the same seed gives the same sentinels [verifies SP-1102~1]", func(t *testing.T) {
			first := sentinelsOf(t, path, source, 7)
			second := sentinelsOf(t, path, source, 7)
			if len(first) == 0 || len(first) != len(second) {
				t.Fatalf("%d and %d sentinels", len(first), len(second))
			}
			for i := range first {
				if first[i].Edit != second[i].Edit {
					t.Fatalf("sentinel %d differs", i)
				}
			}
		})
		t.Run(path+" injected names are absent from the source and change with the seed [verifies SP-1102~1]", func(t *testing.T) {
			names := map[string]bool{}
			for seed := operator.Seed(1); seed <= 30; seed++ {
				for _, c := range sentinelsOf(t, path, source, seed) {
					for _, name := range injectedLocal.FindAllString(c.Edit.NewSource, -1) {
						if strings.Contains(source, name) {
							t.Fatalf("seed %d injects %q, which the source already holds", seed, name)
						}
						names[name] = true
					}
					if c.Kind == operator.SentinelUnusedConstant && strings.Contains(source, c.Edit.Qualname) {
						t.Fatalf("seed %d injects the constant %q", seed, c.Edit.Qualname)
					}
				}
			}
			if len(names) < 10 {
				t.Fatalf("%d distinct names over 30 seeds", len(names))
			}
		})
		t.Run(path+" every sentinel parses and records the original text of its span [verifies SP-1102~1]", func(t *testing.T) {
			for _, c := range sentinelsOf(t, path, source, operator.LocalSeed) {
				if c.Kind != operator.SentinelUnusedConstant && c.Original != source[c.Replaces.Start:c.Replaces.End] {
					t.Fatalf("original does not match its span: %+v", c)
				}
				if _, err := New().Normalize(path, c.Edit.Target, c.Edit.NewSource); err != nil {
					t.Fatalf("%s does not parse: %v", c.Kind, err)
				}
			}
		})
	}
}
