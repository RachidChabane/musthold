package javascript

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

const (
	typescriptPath = "m.ts"
	functionHead   = "function f(a, b) {\n  "
	functionTail   = "\n}"
)

// splice applies an edit with the production splicer, over the real definition
// locator of the language and this engine as the assignment locator.
func splice(source []byte, edit mutation.Edit) ([]byte, error) {
	splicer := mutation.Splicer{Locator: treesitter.NewLocator(), Assignments: New()}
	return splicer.Splice(edit.File, source, edit)
}

func fileAt(path, source string) mutation.SourceFile {
	return mutation.SourceFile{Path: path, Content: []byte(source)}
}

func file(source string) mutation.SourceFile { return fileAt(typescriptPath, source) }

func candidatesOf(t testing.TB, source string) []operator.Candidate {
	t.Helper()
	got, err := New().Candidates(file(source))
	if err != nil {
		t.Fatalf("Candidates: %v", err)
	}
	return got
}

func sourcesOf(candidates []operator.Candidate, family operator.Family) []string {
	var out []string
	for _, c := range candidates {
		if c.Family == family {
			out = append(out, c.Edit.NewSource)
		}
	}
	return out
}

// inFunction writes a statement list as the body of the function `f`.
func inFunction(body string) string { return functionHead + body + functionTail }

func TestOperatorFamiliesInAFunction(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		family operator.Family
		want   []string
	}{
		{"strict relational", "return a < 1;", operator.FamilyRelational,
			[]string{"return a <= 1;", "return a > 1;", "return a >= 1;", "return a === 1;", "return a !== 1;"}},
		{"loose equality keeps its own set", "return a == 1;", operator.FamilyRelational,
			[]string{"return a < 1;", "return a <= 1;", "return a > 1;", "return a >= 1;", "return a != 1;"}},
		{"arithmetic", "return a + b;", operator.FamilyArithmetic,
			[]string{"return a - b;", "return a * b;", "return a / b;", "return a % b;"}},
		{"exponent and bitwise operators are left alone", "return (a ** b) | (a << b);", operator.FamilyArithmetic, nil},
		{"augmented assignment", "a += b;", operator.FamilyArithmetic,
			[]string{"a -= b;", "a *= b;", "a /= b;", "a %= b;"}},
		{"numeric constant", "return a + 15;", operator.FamilyNumericConstant,
			[]string{"return a + 16;", "return a + 14;", "return a + 150;"}},
		{"float constant", "return a * 0.5;", operator.FamilyNumericConstant,
			[]string{"return a * 1.5;", "return a * (-0.5);", "return a * 5.0;"}},
		{"hexadecimal and exponent forms yield nothing", "return a + 0x10 + 1e3;", operator.FamilyNumericConstant, nil},
		{"a numeric literal type is not a value", "let t: 1 | 2 = 1;", operator.FamilyNumericConstant,
			[]string{"let t: 1 | 2 = 2;", "let t: 1 | 2 = 0;", "let t: 1 | 2 = 10;"}},
		{"if condition", "if (a && b) {\n    return 1;\n  }\n  return 2;", operator.FamilyBooleanLogical,
			[]string{
				"if (!(a && b)) {\n    return 1;\n  }\n  return 2;",
				"if (!(a) && b) {\n    return 1;\n  }\n  return 2;",
				"if (a && !(b)) {\n    return 1;\n  }\n  return 2;",
			}},
		{"while and ternary", "while (a) {\n    a = a ? 0 : b;\n  }", operator.FamilyBooleanLogical,
			[]string{"while (!(a)) {\n    a = a ? 0 : b;\n  }", "while (a) {\n    a = !(a) ? 0 : b;\n  }"}},
		{"do while", "do {\n    a--;\n  } while (a);", operator.FamilyBooleanLogical,
			[]string{"do {\n    a--;\n  } while (!(a));"}},
		{"for condition", "for (let i = 0; i < a; i++) {}", operator.FamilyBooleanLogical,
			[]string{"for (let i = 0; !(i < a); i++) {}"}},
		{"and or", "return a || b;", operator.FamilyAndOr, []string{"return a && b;"}},
		{"nullish coalescing is not a logical operator", "return a ?? b;", operator.FamilyAndOr, nil},
		{"return deletion and return undefined", "if (a) {\n    return 1;\n  }\n  return a;", operator.FamilyReturnValue,
			[]string{
				"if (a) {\n    ;\n  }\n  return a;",
				"if (a) {\n    return undefined;\n  }\n  return a;",
				"if (a) {\n    return 1;\n  }\n  return undefined;",
			}},
		{"return as a bare branch body", "if (a) return 1;\n  else return 2;", operator.FamilyReturnValue,
			[]string{
				"if (a) ;\n  else return 2;",
				"if (a) return undefined;\n  else return 2;",
				"if (a) return 1;\n  else ;",
				"if (a) return 1;\n  else return undefined;",
			}},
		{"a returned undefined is not replaced", "return undefined;", operator.FamilyReturnValue, nil},
		{"constructor class call", "return Money(5);", operator.FamilyConstructorArgument,
			[]string{"return Money(6);", "return Money(4);", "return Money(50);"}},
		{"construction", "return new Money(5, a);", operator.FamilyConstructorArgument,
			[]string{"return new Money(6, a);", "return new Money(4, a);", "return new Money(50, a);"}},
		{"a call of a lowercase function is not a constructor", "return round(5);", operator.FamilyConstructorArgument, nil},
		{"min max", "return Math.min(a, Math.max(a, 0));", operator.FamilyMinMax,
			[]string{"return Math.max(a, Math.max(a, 0));", "return Math.min(a, Math.min(a, 0));"}},
		{"min and max of another object are left alone", "return stats.min(a);", operator.FamilyMinMax, nil},
		{"boolean literal", "let ok = true;\n  return false;", operator.FamilyBooleanLiteral,
			[]string{"let ok = false;\n  return false;", "let ok = true;\n  return true;"}},
		{"a literal type is not a value", "let t: true = true;", operator.FamilyBooleanLiteral, []string{"let t: true = false;"}},
		{"thrown class", "throw new TypeError(a);", operator.FamilyExceptionClass, []string{"throw new RangeError(a);"}},
		{"thrown class called without new", "throw CustomError(a);", operator.FamilyExceptionClass, []string{"throw Error(a);"}},
		{"exception suffix", "throw new PricingException(a);", operator.FamilyExceptionClass, []string{"throw new Error(a);"}},
		{"a class that is not an error is left alone", "throw new Foo(a);", operator.FamilyExceptionClass, nil},
		{"a rethrown value is left alone", "throw a;", operator.FamilyExceptionClass, nil},
		{"instanceof", "return a instanceof ValidationError;", operator.FamilyExceptionClass,
			[]string{"return a instanceof Error;"}},
	}
	for _, tt := range tests {
		t.Run(tt.name+" [verifies SP-1000~1]", func(t *testing.T) {
			got := sourcesOf(candidatesOf(t, inFunction(tt.body)+"\n"), tt.family)
			var want []string
			for _, body := range tt.want {
				want = append(want, inFunction(body))
			}
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("got\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestOperatorFamiliesAtModuleLevel(t *testing.T) {
	tests := []struct {
		name   string
		source string
		family operator.Family
		want   []string
	}{
		{"module constant", "const RATE = 15;\n", operator.FamilyModuleConstant, []string{"RATE = 16", "RATE = 14", "RATE = 150"}},
		{"module constant with a type annotation", "export const RATE: number = 15;\n", operator.FamilyModuleConstant,
			[]string{"RATE: number = 16", "RATE: number = 14", "RATE: number = 150"}},
		{"an arrow function value holds numeric constants", "const half = (x: number) => x / 2;\n", operator.FamilyNumericConstant,
			[]string{"half = (x: number) => x / 3", "half = (x: number) => x / 1", "half = (x: number) => x / 20"}},
		{"an arrow function value holds no module constant", "const half = (x: number) => x / 2;\n", operator.FamilyModuleConstant, nil},
		{"a function expression value holds numeric constants", "const twice = function (x) { return x * 2; };\n", operator.FamilyNumericConstant,
			[]string{"twice = function (x) { return x * 3; }", "twice = function (x) { return x * 1; }", "twice = function (x) { return x * 20; }"}},
		{"constructor argument of a declarator", "const GRACE = new Money(15, { scale: 2 });\n", operator.FamilyConstructorArgument,
			[]string{
				"GRACE = new Money(16, { scale: 2 })", "GRACE = new Money(14, { scale: 2 })", "GRACE = new Money(150, { scale: 2 })",
				"GRACE = new Money(15, { scale: 3 })", "GRACE = new Money(15, { scale: 1 })", "GRACE = new Money(15, { scale: 20 })",
			}},
		{"decimal string", "const PRICE = new Decimal(\"0.15\");\n", operator.FamilyDecimalString,
			[]string{"PRICE = new Decimal(\"0.16\")", "PRICE = new Decimal(\"0.14\")", "PRICE = new Decimal(\"1.5\")"}},
		{"decimal string of a call and of Big", "const A = Decimal(\"2\");\nconst B = new Big('3');\n", operator.FamilyDecimalString,
			[]string{
				"A = Decimal(\"3\")", "A = Decimal(\"1\")", "A = Decimal(\"20\")",
				"B = new Big('4')", "B = new Big('2')", "B = new Big('30')",
			}},
		{"a decimal built from a variable is left alone", "const PRICE = new Decimal(raw);\n", operator.FamilyDecimalString, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name+" [verifies SP-1000~1]", func(t *testing.T) {
			got := sourcesOf(candidatesOf(t, tt.source), tt.family)
			want := slices.Clone(tt.want)
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("got\n%q\nwant\n%q", got, want)
			}
		})
	}
}

func TestUnits(t *testing.T) {
	t.Run("an edit targets the outermost definition and a method by its qualified name [verifies SP-1002~1]", func(t *testing.T) {
		source := "class A {\n  m() {\n    function inner(x) {\n      return x < 2;\n    }\n    return inner(1);\n  }\n}\n"
		got := candidatesOf(t, source)
		if len(got) == 0 {
			t.Fatal("no candidate")
		}
		want := ecosystem.Span{Start: strings.Index(source, "m()"), End: strings.LastIndex(source, "\n}")}
		for _, c := range got {
			if c.Edit.Qualname != "A.m" || c.Edit.Target != mutation.TargetDefinition || c.Replaces != want {
				t.Fatalf("candidate %+v", c)
			}
		}
	})

	t.Run("the span of an exported function leaves the export keyword and the terminator outside [verifies SP-1002~1]", func(t *testing.T) {
		source := "export function f(a) {\n  return a < 1;\n}\nexport const K = 1;\n"
		for _, c := range candidatesOf(t, source) {
			switch c.Edit.Qualname {
			case "f":
				if !strings.HasPrefix(c.Original, "function f") || !strings.HasSuffix(c.Original, "}") {
					t.Fatalf("definition unit %q", c.Original)
				}
			case "K":
				if c.Original != "K = 1" {
					t.Fatalf("assignment unit %q", c.Original)
				}
			}
		}
	})

	t.Run("a declarator is a unit when it is written at the top of the module only and holds no definition [verifies SP-1002~1]", func(t *testing.T) {
		source := `const A = 1;
export const B: number = 2, C = 3;
let { D } = obj;
const [E] = arr;
if (A) { var F = 5; }
class K { static G = 6; }
function h() { const H = 7; }
const I = function () { return 8; };
const J = { m() { return 9; } };
const L = class { m() {} };
let M;
`
		assignments, err := New().LocateAssignments(typescriptPath, []byte(source))
		if err != nil {
			t.Fatal(err)
		}
		var qualnames []string
		for _, a := range assignments {
			if a.Kind != ecosystem.KindAssignment || source[a.Span.Start:a.Span.End] == "" {
				t.Fatalf("assignment %+v", a)
			}
			qualnames = append(qualnames, a.Qualname)
		}
		if want := []string{"A", "B", "C", "I"}; !slices.Equal(qualnames, want) {
			t.Fatalf("assignment units = %v, want %v", qualnames, want)
		}
	})

	t.Run("a unit whose kind and qualname are shared is left out [verifies SP-1002~1]", func(t *testing.T) {
		source := "class A {\n  get v() { return 1; }\n  set v(x) { this.x = x < 2; }\n}\nfunction f(a) { return a < 1; }\nconst X = 1;\nvar X = 2;\nconst Y = 3;\n"
		qualnames := map[string]bool{}
		for _, c := range candidatesOf(t, source) {
			qualnames[string(c.Edit.Target)+":"+c.Edit.Qualname] = true
		}
		want := map[string]bool{"definition:f": true, "module_assignment:Y": true}
		if len(qualnames) != len(want) || !qualnames["definition:f"] || !qualnames["module_assignment:Y"] {
			t.Fatalf("units = %v", qualnames)
		}
	})

	t.Run("a definition nested in a class expression is named like the shared locator names it [verifies SP-1002~1]", func(t *testing.T) {
		source := "function make() {\n  return class {\n    area() { return 1 < 2; }\n  };\n}\nfunction area() { return 3 < 4; }\n"
		located, err := treesitter.NewLocator().Locate(typescriptPath, []byte(source))
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		for _, d := range located {
			want = append(want, d.Qualname)
		}
		if !slices.Contains(want, "make.area") {
			t.Fatalf("locator qualnames = %v", want)
		}
		for _, c := range candidatesOf(t, source) {
			if !slices.Contains(want, c.Edit.Qualname) {
				t.Fatalf("qualname %q unknown to the locator %v", c.Edit.Qualname, want)
			}
		}
	})

	t.Run("the site span covers the mutated node in the original file [verifies SP-1000~1]", func(t *testing.T) {
		source := "function f(a) {\n  return a < 10;\n}\n"
		for _, c := range candidatesOf(t, source) {
			text := source[c.Site.Span.Start:c.Site.Span.End]
			if c.Family == operator.FamilyRelational && text != "<" {
				t.Fatalf("site text %q", text)
			}
		}
	})

	t.Run("a source with a syntax error is refused [verifies SP-1002~1]", func(t *testing.T) {
		if _, err := New().Candidates(file("function f(:\n")); !errors.Is(err, ErrSyntax) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a file no grammar covers is refused [verifies SP-1002~1]", func(t *testing.T) {
		if _, err := New().Candidates(fileAt("m.rb", "x = 1\n")); !errors.Is(err, ErrUnsupportedFile) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("the engine declares every family and its extensions [verifies SP-1000~1]", func(t *testing.T) {
		if len(New().Families()) != len(operator.Families()) {
			t.Fatal("families missing")
		}
		want := []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx"}
		if got := New().Extensions(); !slices.Equal(got, want) {
			t.Fatalf("extensions = %v", got)
		}
	})
}

func TestGrammarFollowsTheExtension(t *testing.T) {
	typed := "function f(a: number): number {\n  return a < 1 ? 1 : 2;\n}\n"
	jsx := "export function View({ n }: { n: number }) {\n  return <p>{n < 3 ? 1 : 2}</p>;\n}\n"
	plain := "function f(a) {\n  const b = a < 1 ? 1 : 2;\n  return b;\n}\nconst RATE = 15;\n"

	t.Run("a type annotation parses in TypeScript and not in JavaScript [verifies SP-1002~1]", func(t *testing.T) {
		if _, err := New().Candidates(fileAt("m.ts", typed)); err != nil {
			t.Fatal(err)
		}
		if _, err := New().Candidates(fileAt("m.js", typed)); !errors.Is(err, ErrSyntax) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("markup parses in TSX and not in TypeScript [verifies SP-1002~1]", func(t *testing.T) {
		if _, err := New().Candidates(fileAt("m.tsx", jsx)); err != nil {
			t.Fatal(err)
		}
		if _, err := New().Candidates(fileAt("m.ts", jsx)); !errors.Is(err, ErrSyntax) {
			t.Fatalf("err = %v", err)
		}
	})

	for _, path := range []string{"m.js", "m.jsx", "m.mjs", "m.cjs", "m.ts", "m.tsx"} {
		t.Run(path+" every candidate and sentinel splices into a parsable file [verifies SP-1005~1]", func(t *testing.T) {
			source := fileAt(path, plain)
			assertEverySpliceParses(t, source)
		})
	}

	t.Run("a TSX component is mutated and spliced [verifies SP-1005~1]", func(t *testing.T) {
		source := fileAt("view.tsx", jsx)
		if len(candidatesOfFile(t, source)) == 0 {
			t.Fatal("no candidate")
		}
		assertEverySpliceParses(t, source)
	})
}

func candidatesOfFile(t testing.TB, source mutation.SourceFile) []operator.Candidate {
	t.Helper()
	got, err := New().Candidates(source)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// assertEverySpliceParses splices every candidate and sentinel of a file with
// the production splicer and re-parses the result.
func assertEverySpliceParses(t testing.TB, source mutation.SourceFile) {
	t.Helper()
	candidates := candidatesOfFile(t, source)
	sentinels, err := New().SentinelCandidates(source, operator.LocalSeed)
	if err != nil {
		t.Fatal(err)
	}
	var edits []mutation.Edit
	for _, c := range candidates {
		edits = append(edits, c.Edit)
	}
	for _, s := range sentinels {
		edits = append(edits, s.Edit)
	}
	if len(edits) == 0 {
		t.Fatal("nothing to splice")
	}
	for _, edit := range edits {
		spliced, err := splice(source.Content, edit)
		if err != nil {
			t.Fatalf("%s %s: %v", edit.Target, edit.Qualname, err)
		}
		if _, err := New().CallGraph(mutation.SourceFile{Path: source.Path, Content: spliced}); err != nil {
			t.Fatalf("%s %s gives an unparsable file: %v\n%s", edit.Target, edit.Qualname, err, spliced)
		}
	}
}

func TestATemplateLiteralHoldingANulByte(t *testing.T) {
	t.Run("a function whose template literal holds a NUL byte is mutated like any other [verifies SP-2427~1]", func(t *testing.T) {
		source := "export function pairKey(a: string, b: string): string {\n  return a < b ? `${a}\x00${b}` : `${b}\x00${a}`;\n}\n"
		if got := sourcesOf(candidatesOf(t, source), operator.FamilyRelational); len(got) == 0 {
			t.Fatal("no relational mutant of pairKey")
		}
	})
}
