package jvm

import (
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

// The sources the definition locator is specified with.
const (
	locatorJavaSource   = "class A {\n  A() {}\n  int m(int x) { return x; }\n  static class B {\n    void n() {}\n  }\n}\n"
	locatorKotlinSource = "class A {\n  fun m(x: Int): Int { return x }\n}\nfun top() {}\nobject O {\n  fun q() {}\n}\n"
)

func TestDefinitionQualnames(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		source string
		want   []string
	}{
		{"java constructor, method and nested class", javaPath, locatorJavaSource, []string{"A.A", "A.m", "A.B.n"}},
		{"kotlin method, top-level function and object", kotlinPath, locatorKotlinSource, []string{"A.m", "top", "O.q"}},
		{"java interface, enum, record and enum constant body", javaPath,
			"interface I { default void d() {} }\nenum E { X { void x() {} }; void e() {} }\nrecord R(int a) { int r() { return a; } }\n",
			[]string{"I.d", "E.x", "E.e", "R.r"}},
		{"kotlin companion object belongs to its class", kotlinPath,
			"class A {\n  companion object {\n    fun q() {}\n  }\n  fun m() {}\n}\n",
			[]string{"A.q", "A.m"}},
		{"kotlin nested classes and named companion", kotlinPath,
			"class A {\n  class B {\n    fun n() {}\n  }\n  companion object Factory {\n    fun make() {}\n  }\n}\n",
			[]string{"A.B.n", "A.make"}},
		{"kotlin extension function and generic function", kotlinPath,
			"fun String.shout(): String = this\nfun <T> first(items: List<T>): T = items[0]\n",
			[]string{"shout", "first"}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" [verifies SP-1101~1]", func(t *testing.T) {
			got := qualnamesOf(unitsOf(t, tt.path, tt.source), mutation.TargetDefinition)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
			located, err := testLocator{}.Locate(tt.path, []byte(tt.source))
			if err != nil {
				t.Fatal(err)
			}
			for _, qualname := range got {
				if !slices.ContainsFunc(located, func(d ecosystem.Definition) bool { return d.Qualname == qualname }) {
					t.Errorf("%s is not a definition of the locator", qualname)
				}
			}
		})
	}
}

func TestNestedDefinitionsBelongToTheOutermost(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		source string
		want   []string
	}{
		{"java anonymous class, local class and lambda", javaPath,
			"class T {\n  int m(int a) {\n    Runnable r = new Runnable() { public void run() { int b = a < 2 ? 1 : 0; } };\n    class Local { int l() { return a > 1 ? 1 : 0; } }\n    java.util.function.IntSupplier s = () -> a < 3 ? 1 : 0;\n    return a;\n  }\n}\n",
			[]string{"T.m"}},
		{"kotlin local function, object literal and lambda", kotlinPath,
			"fun top(a: Int): Int {\n  fun inner(b: Int) = if (b < 2) 1 else 0\n  val o = object : Runnable { override fun run() { val c = if (a > 1) 1 else 0 } }\n  val l = { x: Int -> if (x < 3) 1 else 0 }\n  return inner(a)\n}\n",
			[]string{"top"}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" [verifies SP-1101~1]", func(t *testing.T) {
			if got := qualnamesOf(unitsOf(t, tt.path, tt.source), mutation.TargetDefinition); !slices.Equal(got, tt.want) {
				t.Fatalf("units %v want %v", got, tt.want)
			}
			relational := 0
			for _, c := range candidatesOf(t, tt.path, tt.source) {
				if c.Edit.Qualname != tt.want[0] {
					t.Fatalf("candidate of %s", c.Edit.Qualname)
				}
				if c.Family == operator.FamilyRelational {
					relational++
				}
			}
			if relational != 3*5 {
				t.Fatalf("%d relational mutants, the nested comparisons are not all offered", relational)
			}
		})
	}
}

func TestOverloadsAreLeftOut(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		source string
		want   []string
	}{
		{"java overloaded methods and constructors", javaPath,
			"class T {\n  T(int a) { if (a < 1) { a = 2; } }\n  T(int a, int b) { if (a < 1) { a = 2; } }\n  int f(int a) { return a < 1 ? 1 : 0; }\n  int f(int a, int b) { return a < 1 ? 1 : 0; }\n  int g(int a) { return a < 1 ? 1 : 0; }\n}\n",
			[]string{"T.g"}},
		{"kotlin overloaded functions", kotlinPath,
			"fun f(a: Int): Int = if (a < 1) 1 else 0\nfun f(a: Int, b: Int): Int = if (a < 1) 1 else 0\nfun g(a: Int): Int = if (a < 1) 1 else 0\n",
			[]string{"g"}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" [verifies SP-1101~1]", func(t *testing.T) {
			units := unitsOf(t, tt.path, tt.source)
			if got := qualnamesOf(units, mutation.TargetDefinition); !slices.Equal(got, tt.want) {
				t.Fatalf("units %v want %v", got, tt.want)
			}
			for _, c := range candidatesOf(t, tt.path, tt.source) {
				if c.Edit.Qualname != tt.want[0] {
					t.Fatalf("candidate of the overloaded %s", c.Edit.Qualname)
				}
			}
			for _, s := range sentinelsOf(t, tt.path, tt.source, operator.LocalSeed) {
				if s.Edit.Target == mutation.TargetDefinition && s.Edit.Qualname != tt.want[0] {
					t.Fatalf("sentinel of the overloaded %s", s.Edit.Qualname)
				}
			}
			graph, err := New().CallGraph(sourceFile(tt.path, tt.source))
			if err != nil {
				t.Fatal(err)
			}
			if len(graph.Nodes) != 1 || graph.Nodes[0].Qualname != tt.want[0] {
				t.Fatalf("nodes %+v", graph.Nodes)
			}
		})
	}
}

func TestAssignmentUnits(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		source string
		want   []string
	}{
		{"java static final fields, interface constants and nested owners", javaPath,
			"class Outer {\n  static final int A = 1;\n  public static final String B = \"b\";\n  static class Inner { static final int C = 3; }\n  interface Limits { int D = 4; }\n  enum Mode { X; static final int E = 5; }\n}\n",
			[]string{"Outer.A", "Outer.B", "Outer.Inner.C", "Outer.Limits.D", "Outer.Mode.E"}},
		{"java fields that are not constants are no unit", javaPath,
			"class T {\n  final int a = 1;\n  static int b = 2;\n  static final int c;\n  static final int d = 3, e = 4;\n  static final int f = 5;\n  void m() { final int g = 6; }\n}\n",
			[]string{"T.f"}},
		{"java field initialised with an anonymous class is no unit", javaPath,
			"class T {\n  static final Runnable R = new Runnable() { public void run() { } };\n  static final int K = 1;\n}\n",
			[]string{"T.K"}},
		{"kotlin const and val properties at the top level", kotlinPath,
			"const val TOP = 5\nval RATE: Int = 15\nvar mutable = 1\nval computed: Int get() = 3\nval lazyValue by lazy { 1 }\nval noInitializer: Int\n",
			[]string{"TOP", "RATE"}},
		{"kotlin properties of objects, companions and classes", kotlinPath,
			"object O { val X = 1 }\nclass C {\n  val z = 4\n  var w = 5\n  companion object { const val K = 3 }\n  fun m() { val local = 2 }\n}\n",
			[]string{"O.X", "C.z", "C.K"}},
		{"kotlin property followed by its accessor is no unit", kotlinPath,
			"class C {\n  val a = 1\n    get() = field + 1\n  val b = 2\n}\n",
			[]string{"C.b"}},
		{"a name shared by a property and a companion property is left out", kotlinPath,
			"class C {\n  val K = 1\n  companion object { val K = 2 }\n  val J = 3\n}\n",
			[]string{"C.J"}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" [verifies SP-1101~1]", func(t *testing.T) {
			got := qualnamesOf(unitsOf(t, tt.path, tt.source), mutation.TargetModuleAssignment)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestLocateAssignments(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		source string
		spans  map[string]string
	}{
		{"java field span covers modifiers and semicolon", javaPath,
			"class T {\n  public static final int A = 1;\n  void m() {}\n}\n",
			map[string]string{"T.A": "public static final int A = 1;"}},
		{"kotlin property span covers modifiers", kotlinPath,
			"const val TOP = 5\nclass C { val z = 4 }\n",
			map[string]string{"TOP": "const val TOP = 5", "C.z": "val z = 4"}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" [verifies SP-1101~1]", func(t *testing.T) {
			located, err := New().LocateAssignments(tt.path, []byte(tt.source))
			if err != nil {
				t.Fatal(err)
			}
			if len(located) != len(tt.spans) {
				t.Fatalf("located %+v", located)
			}
			for _, d := range located {
				text, err := d.Span.Slice([]byte(tt.source))
				if err != nil || d.Kind != ecosystem.KindAssignment || string(text) != tt.spans[d.Qualname] {
					t.Fatalf("%+v text %q", d, text)
				}
			}
		})
	}
}

func TestAssignmentCandidatesTargetTheDeclaration(t *testing.T) {
	t.Run("a constant mutant is a module assignment edit of the whole declaration [verifies SP-1101~1]", func(t *testing.T) {
		source := "class T {\n  static final int RATE = 15;\n}\n"
		got := candidatesOf(t, javaPath, source)
		if len(got) != 3 {
			t.Fatalf("candidates %d", len(got))
		}
		for _, c := range got {
			if c.Edit.Target != mutation.TargetModuleAssignment || c.Edit.Qualname != "T.RATE" || c.Original != "static final int RATE = 15;" || c.Family != operator.FamilyModuleConstant {
				t.Fatalf("candidate %+v", c)
			}
		}
	})
}

func TestDefinitionSpanCoversAnnotationsAndModifiers(t *testing.T) {
	t.Run("a unit starts at its annotation and ends at its closing brace [verifies SP-1101~1]", func(t *testing.T) {
		source := "class T {\n  @Override\n  public int hashCode() { return 1; }\n}\n"
		units := unitsOf(t, javaPath, source)
		if len(units) != 1 || units[0].text != "@Override\n  public int hashCode() { return 1; }" {
			t.Fatalf("units %+v", units)
		}
	})
}

func TestSourceWithASyntaxErrorIsRefused(t *testing.T) {
	engine := New()
	broken := map[string]string{javaPath: "class T { int f( { }", kotlinPath: "fun f(: Int {"}
	for path, source := range broken {
		t.Run(path+" [verifies SP-1101~1]", func(t *testing.T) {
			file := sourceFile(path, source)
			checks := map[string]func() error{
				"Candidates":         func() error { _, err := engine.Candidates(file); return err },
				"SentinelCandidates": func() error { _, err := engine.SentinelCandidates(file, operator.LocalSeed); return err },
				"CallGraph":          func() error { _, err := engine.CallGraph(file); return err },
				"LocateAssignments":  func() error { _, err := engine.LocateAssignments(path, []byte(source)); return err },
			}
			for name, check := range checks {
				err := check()
				if !errors.Is(err, ErrSyntax) || err.Error() == ErrSyntax.Error() {
					t.Errorf("%s: err = %v, want the path wrapped around ErrSyntax", name, err)
				}
			}
		})
	}
	t.Run("an extension without a dialect is refused [verifies SP-1101~1]", func(t *testing.T) {
		if _, err := engine.Candidates(sourceFile("src/T.scala", "object T")); !errors.Is(err, ErrUnsupportedExtension) {
			t.Fatalf("err = %v", err)
		}
	})
}
