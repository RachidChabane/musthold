package jvm

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

// rewrite replaces the first occurrence of from in a unit by to.
type rewrite struct{ from, to string }

// familyCase is a unit and the exact set of mutated units one family yields.
type familyCase struct {
	name   string
	path   string
	unit   string
	family operator.Family
	want   []rewrite
}

func (c familyCase) wanted() []string {
	var out []string
	for _, r := range c.want {
		out = append(out, strings.Replace(c.unit, r.from, r.to, 1))
	}
	slices.Sort(out)
	return out
}

func (c familyCase) source() string {
	if c.path == javaPath {
		return javaFile(c.unit)
	}
	return kotlinFile(c.unit)
}

func java(name, unit string, family operator.Family, want ...rewrite) familyCase {
	return familyCase{name: "java " + name, path: javaPath, unit: unit, family: family, want: want}
}

func kotlin(name, unit string, family operator.Family, want ...rewrite) familyCase {
	return familyCase{name: "kotlin " + name, path: kotlinPath, unit: unit, family: family, want: want}
}

func rw(from string, to ...string) []rewrite {
	out := make([]rewrite, 0, len(to))
	for _, replacement := range to {
		out = append(out, rewrite{from, replacement})
	}
	return out
}

func join(groups ...[]rewrite) []rewrite { return slices.Concat(groups...) }

func operatorCases() []familyCase {
	var cases []familyCase
	cases = append(cases, relationalCases()...)
	cases = append(cases, arithmeticCases()...)
	cases = append(cases, constantCases()...)
	cases = append(cases, logicalCases()...)
	cases = append(cases, callCases()...)
	cases = append(cases, returnCases()...)
	cases = append(cases, exceptionCases()...)
	return cases
}

func relationalCases() []familyCase {
	return []familyCase{
		java("relational", "static boolean f(int a) { return a < 1; }", operator.FamilyRelational,
			rw("a < 1", "a <= 1", "a > 1", "a >= 1", "a == 1", "a != 1")...),
		java("equality only swaps with its negation", "static boolean f(int a, int b) { return a == b; }", operator.FamilyRelational,
			rw("a == b", "a != b")...),
		kotlin("relational", "fun f(a: Int): Boolean { return a >= 1 }", operator.FamilyRelational,
			rw("a >= 1", "a < 1", "a <= 1", "a > 1", "a == 1", "a != 1")...),
		kotlin("equality only swaps with its negation", "fun f(a: Int, b: Int): Boolean { return a != b }", operator.FamilyRelational,
			rw("a != b", "a == b")...),
	}
}

func arithmeticCases() []familyCase {
	return []familyCase{
		java("arithmetic", "static int f(int a, int b) { return a + b; }", operator.FamilyArithmetic,
			rw("a + b", "a - b", "a * b", "a / b", "a % b")...),
		java("a string concatenation keeps its plus", `static String f(String a, int b) { return "x" + a + b; }`, operator.FamilyArithmetic),
		java("a parenthesized string concatenation keeps its plus", `static String f(String a, int b) { return ("x" + a) + b; }`, operator.FamilyArithmetic),
		kotlin("arithmetic", "fun f(a: Int, b: Int): Int { return a % b }", operator.FamilyArithmetic,
			rw("a % b", "a + b", "a - b", "a * b", "a / b")...),
		kotlin("a string concatenation keeps its plus", `fun f(a: String, b: Int): String { return "x" + a + b }`, operator.FamilyArithmetic),
		kotlin("a string template has no operator", `fun f(a: String): String { return "x$a" }`, operator.FamilyArithmetic),
	}
}

func constantCases() []familyCase {
	return []familyCase{
		java("numeric constant", "static int f(int a) { return a + 15; }", operator.FamilyNumericConstant,
			rw("a + 15", "a + 16", "a + 14", "a + 150")...),
		java("long suffix is kept", "static long f(long a) { return a * 15L; }", operator.FamilyNumericConstant,
			rw("15L", "16L", "14L", "150L")...),
		java("a negative shift carries the suffix inside the parentheses", "static long f() { return 0L; }", operator.FamilyNumericConstant,
			rw("0L", "1L", "(-1L)")...),
		java("float suffix is kept", "static float f(float a) { return a * 1.5f; }", operator.FamilyNumericConstant,
			rw("1.5f", "2.5f", "0.5f", "15.0f")...),
		java("hexadecimal, separated and exponent forms are left alone", "static long f() { return 0x1F + 1_000 + 1e3; }", operator.FamilyNumericConstant),
		java("module constant", "static final int RATE = 15;", operator.FamilyModuleConstant,
			rw("= 15", "= 16", "= 14", "= 150")...),
		java("constructor argument", "static Object f() { return new Money(5); }", operator.FamilyConstructorArgument,
			rw("(5)", "(6)", "(4)", "(50)")...),
		java("factory argument", "static Object f() { return Duration.ofMinutes(15); }", operator.FamilyConstructorArgument,
			rw("(15)", "(16)", "(14)", "(150)")...),
		java("decimal string", `static BigDecimal f() { return new BigDecimal("4.50"); }`, operator.FamilyDecimalString,
			rw("4.50", "4.51", "4.49", "45.0")...),
		kotlin("numeric constant", "fun f(a: Int): Int { return a + 15 }", operator.FamilyNumericConstant,
			rw("a + 15", "a + 16", "a + 14", "a + 150")...),
		kotlin("long suffix is kept", "fun f(a: Long): Long { return a * 15L }", operator.FamilyNumericConstant,
			rw("15L", "16L", "14L", "150L")...),
		kotlin("float suffix is kept", "fun f(a: Float): Float { return a * 1.5f }", operator.FamilyNumericConstant,
			rw("1.5f", "2.5f", "0.5f", "15.0f")...),
		kotlin("unsigned, hexadecimal and separated forms are left alone", "fun f(): Long { return 5u + 0x1F + 1_000 }", operator.FamilyNumericConstant),
		kotlin("module constant", "const val RATE = 15", operator.FamilyModuleConstant,
			rw("= 15", "= 16", "= 14", "= 150")...),
		kotlin("constructor argument", "fun f(): Any { return Money(5) }", operator.FamilyConstructorArgument,
			rw("(5)", "(6)", "(4)", "(50)")...),
		kotlin("factory argument", "fun f(): Any { return Duration.ofMinutes(15) }", operator.FamilyConstructorArgument,
			rw("(15)", "(16)", "(14)", "(150)")...),
		kotlin("decimal string", `fun f(): BigDecimal { return BigDecimal("4.50") }`, operator.FamilyDecimalString,
			rw("4.50", "4.51", "4.49", "45.0")...),
	}
}

func logicalCases() []familyCase {
	return []familyCase{
		java("boolean logical on an if and its operands", "static int f(boolean a, boolean b) { if (a && b) { return 1; } return 2; }", operator.FamilyBooleanLogical,
			rw("if (a && b)", "if (!(a && b))", "if (!(a) && b)", "if (a && !(b))")...),
		java("boolean logical on loops and a ternary", "static int f(int a) { while (a > 0) { a--; } do { a++; } while (a < 3); return a > 1 ? 1 : 0; }", operator.FamilyBooleanLogical,
			join(rw("while (a > 0)", "while (!(a > 0))"), rw("while (a < 3)", "while (!(a < 3))"), rw("return a > 1", "return !(a > 1)"))...),
		java("boolean logical on a for condition", "static void f(int a) { for (int i = 0; i < a; i++) { } }", operator.FamilyBooleanLogical,
			rw("i < a;", "!(i < a);")...),
		java("and or", "static boolean f(boolean a, boolean b) { return a || b; }", operator.FamilyAndOr,
			rw("a || b", "a && b")...),
		java("boolean literal", "static boolean f() { boolean x = true; return x; }", operator.FamilyBooleanLiteral,
			rw("= true", "= false")...),
		kotlin("boolean logical on an if and its operands", "fun f(a: Boolean, b: Boolean): Int { if (a && b) { return 1 }; return 2 }", operator.FamilyBooleanLogical,
			rw("if (a && b)", "if (!(a && b))", "if (!(a) && b)", "if (a && !(b))")...),
		kotlin("boolean logical on loops and an if expression", "fun f(a: Int): Int { var x = a; while (x > 0) { x-- }; do { x++ } while (x < 3); return if (x > 1) 1 else 0 }", operator.FamilyBooleanLogical,
			join(rw("while (x > 0)", "while (!(x > 0))"), rw("while (x < 3)", "while (!(x < 3))"), rw("if (x > 1)", "if (!(x > 1))"))...),
		kotlin("and or", "fun f(a: Boolean, b: Boolean): Boolean { return a || b }", operator.FamilyAndOr,
			rw("a || b", "a && b")...),
		kotlin("boolean literal", "fun f(): Boolean { val x = false; return x }", operator.FamilyBooleanLiteral,
			rw("= false", "= true")...),
	}
}

func callCases() []familyCase {
	return []familyCase{
		java("Math min", "static int f(int a) { return Math.min(a, 1); }", operator.FamilyMinMax,
			rw("Math.min", "Math.max")...),
		java("receiver max", "static BigDecimal f(BigDecimal a, BigDecimal b) { return a.max(b); }", operator.FamilyMinMax,
			rw("a.max", "a.min")...),
		kotlin("minOf", "fun f(a: Int): Int { return minOf(a, 1) }", operator.FamilyMinMax,
			rw("minOf", "maxOf")...),
		kotlin("kotlin math max", "fun f(a: Int): Int { return max(a, 1) }", operator.FamilyMinMax,
			rw("max", "min")...),
		kotlin("Math min", "fun f(a: Int): Int { return Math.min(a, 1) }", operator.FamilyMinMax,
			rw("Math.min", "Math.max")...),
	}
}

func returnCases() []familyCase {
	return []familyCase{
		java("return deletion and default of an int", "static int f(int a) { if (a > 0) { return 1; } return a; }", operator.FamilyReturnValue,
			join(rw("{ return 1; }", "{ ; }"), rw("return 1;", "return 0;"), rw("return a;", "return 0;"))...),
		java("return default of a boolean", "static boolean f(int a) { return a > 0; }", operator.FamilyReturnValue,
			rw("return a > 0;", "return false;")...),
		java("return default of a char", "static char f() { return 'x'; }", operator.FamilyReturnValue,
			rw("'x'", `'\0'`)...),
		java("return default of a reference", `static String f() { return "x"; }`, operator.FamilyReturnValue,
			rw(`"x"`, "null")...),
		java("a void return and a lambda return have no default", "static void f(java.util.function.IntSupplier s) { s = () -> { return 1; }; return; }", operator.FamilyReturnValue),
		kotlin("return default of an int and a branch value", "fun f(a: Int): Int { if (a > 0) { return 1 }; return a }", operator.FamilyReturnValue,
			join(rw("return 1 }", "return 0 }"), rw("return a }", "return 0 }"))...),
		kotlin("return default of a double", "fun f(a: Double): Double { return a * 2.0 }", operator.FamilyReturnValue,
			rw("return a * 2.0", "return 0.0")...),
		kotlin("return default of a float", "fun f(a: Float): Float { return a * 2f }", operator.FamilyReturnValue,
			rw("return a * 2f", "return 0.0f")...),
		kotlin("return default of a boolean", "fun f(a: Int): Boolean { return a > 0 }", operator.FamilyReturnValue,
			rw("return a > 0", "return false")...),
		kotlin("return default of a nullable", `fun f(a: Int): String? { return "x" }`, operator.FamilyReturnValue,
			rw(`return "x"`, "return null")...),
		kotlin("a non nullable reference has no default", `fun f(a: Int): String { return "x" }`, operator.FamilyReturnValue),
		kotlin("a lambda return and a labeled return have no default", "fun f(a: List<Int>): Int { a.forEach { if (it > 0) { return@forEach } }; a.forEach { return 3 }; return 2 }", operator.FamilyReturnValue,
			rw("return 2 }", "return 0 }")...),
	}
}

func exceptionCases() []familyCase {
	return []familyCase{
		java("thrown and caught classes", `static void f() { try { throw new IllegalStateException("x"); } catch (RuntimeException e) { } }`, operator.FamilyExceptionClass,
			join(rw("new IllegalStateException", "new IllegalArgumentException"), rw("catch (RuntimeException e)", "catch (IllegalStateException e)"))...),
		java("custom and qualified classes become runtime exceptions", `static void f() { try { throw new PricingError("x"); } catch (java.io.IOException e) { } }`, operator.FamilyExceptionClass,
			join(rw("new PricingError", "new RuntimeException"), rw("catch (java.io.IOException e)", "catch (RuntimeException e)"))...),
		java("multi catch", `static void f() { try { g(); } catch (NullPointerException | ArithmeticException e) { } }`, operator.FamilyExceptionClass,
			join(rw("NullPointerException |", "IllegalArgumentException |"), rw("| ArithmeticException", "| IllegalArgumentException"))...),
		java("a name that is not an exception class is left alone", `static void f() { throw new Failure(); }`, operator.FamilyExceptionClass),
		kotlin("thrown and caught classes", `fun f() { try { throw IllegalStateException("x") } catch (e: RuntimeException) { } }`, operator.FamilyExceptionClass,
			join(rw("throw IllegalStateException", "throw IllegalArgumentException"), rw("e: RuntimeException", "e: IllegalStateException"))...),
		kotlin("custom classes become runtime exceptions", `fun f() { try { throw PricingError("x") } catch (e: IndexOutOfBoundsException) { } }`, operator.FamilyExceptionClass,
			join(rw("throw PricingError", "throw RuntimeException"), rw("e: IndexOutOfBoundsException", "e: NoSuchElementException"))...),
		kotlin("a name that is not an exception class is left alone", `fun f() { throw Failure() }`, operator.FamilyExceptionClass),
	}
}

func TestOperatorFamilies(t *testing.T) {
	for _, tt := range operatorCases() {
		t.Run(tt.name+" ["+string(tt.family)+"] [verifies SP-1100~1]", func(t *testing.T) {
			got := sourcesOf(candidatesOf(t, tt.path, tt.source()), tt.family)
			want := tt.wanted()
			if !slices.Equal(got, want) {
				t.Fatalf("got\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestEveryFamilyHasAJavaAndAKotlinCase(t *testing.T) {
	t.Run("every family the engine declares is exercised in both dialects [verifies SP-1100~1]", func(t *testing.T) {
		covered := map[string]map[operator.Family]bool{javaPath: {}, kotlinPath: {}}
		for _, tt := range operatorCases() {
			if len(tt.want) > 0 {
				covered[tt.path][tt.family] = true
			}
		}
		for _, family := range New().Families() {
			for path, families := range covered {
				if !families[family] {
					t.Errorf("%s: no case for %s", path, family)
				}
			}
		}
	})
}

func TestCandidatesCarryTheirUnit(t *testing.T) {
	source := javaFile("static int f(int a) { return a < 10 ? 1 : 2; }")
	t.Run("a candidate replaces its whole unit and the site covers the mutated node [verifies SP-1100~1]", func(t *testing.T) {
		candidates := candidatesOf(t, javaPath, source)
		if len(candidates) == 0 {
			t.Fatal("no candidate")
		}
		for _, c := range candidates {
			unitText := source[c.Replaces.Start:c.Replaces.End]
			if c.Original != unitText || c.Edit.Qualname != "T.f" || c.Edit.Target != mutation.TargetDefinition || c.Site.Qualname != "T.f" {
				t.Fatalf("candidate %+v", c)
			}
			if c.Family == operator.FamilyRelational && source[c.Site.Span.Start:c.Site.Span.End] != "<" {
				t.Fatalf("site text %q", source[c.Site.Span.Start:c.Site.Span.End])
			}
		}
	})
	t.Run("the engine declares its extensions and every family [verifies SP-1100~1]", func(t *testing.T) {
		if !slices.Equal(New().Extensions(), []string{".java", ".kt"}) || len(New().Families()) != len(operator.Families()) {
			t.Fatalf("extensions %v families %v", New().Extensions(), New().Families())
		}
	})
}
