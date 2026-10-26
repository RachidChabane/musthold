package javascript

import (
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

const (
	sampleCap       = 7
	maxStatements   = 3
	expressionDepth = 2
)

var (
	propertyNumbers   = []string{"0", "1", "7", "15", "0.5", "2.25", "100"}
	propertyNames     = []string{"a", "b", "c"}
	propertyLeaves    = []string{"true", "false", `new Decimal("1.50")`, `Decimal("2")`, "new Money(5, c)", "Money(3)", "undefined"}
	propertyOperators = []string{"+", "-", "*", "/", "%", "<", "<=", ">", ">=", "==", "!=", "===", "!==", "&&", "||", "??"}
)

// typescriptFunction draws a small module whose function body exercises the
// operators. Every binary expression is parenthesized, so the source parses
// whatever operators are drawn.
func typescriptFunction(t *rapid.T) string {
	number := rapid.SampledFrom(propertyNumbers)
	name := rapid.SampledFrom(propertyNames)
	var expression func(depth int) *rapid.Generator[string]
	expression = func(depth int) *rapid.Generator[string] {
		leaf := rapid.OneOf(number, name, rapid.SampledFrom(propertyLeaves))
		if depth == 0 {
			return leaf
		}
		sub := expression(depth - 1)
		return rapid.OneOf(
			leaf,
			rapid.Custom(func(t *rapid.T) string {
				op := rapid.SampledFrom(propertyOperators).Draw(t, "op")
				return "(" + sub.Draw(t, "left") + " " + op + " " + sub.Draw(t, "right") + ")"
			}),
			rapid.Custom(func(t *rapid.T) string {
				fn := rapid.SampledFrom([]string{"min", "max"}).Draw(t, "fn")
				return "Math." + fn + "(" + sub.Draw(t, "x") + ", " + sub.Draw(t, "y") + ")"
			}),
			rapid.Custom(func(t *rapid.T) string { return "(!" + leaf.Draw(t, "operand") + ")" }),
			rapid.Custom(func(t *rapid.T) string {
				return "(" + sub.Draw(t, "test") + " ? " + sub.Draw(t, "then") + " : " + sub.Draw(t, "otherwise") + ")"
			}),
		)
	}
	expr := expression(expressionDepth)
	var module strings.Builder
	module.WriteString("import { helper as aid } from \"./aid\";\n")
	module.WriteString("export const RATE = " + number.Draw(t, "rate") + ";\n")
	module.WriteString("const SCALE: number = " + number.Draw(t, "scale") + ";\n\n")
	module.WriteString("export function f(a: number, b: number, c: number) {\n")
	for range rapid.IntRange(0, maxStatements).Draw(t, "statements") {
		switch rapid.IntRange(0, 5).Draw(t, "kind") {
		case 0:
			module.WriteString("  if (" + expr.Draw(t, "cond") + ") {\n    return " + expr.Draw(t, "value") + ";\n  }\n")
		case 1:
			module.WriteString("  const local = " + expr.Draw(t, "assigned") + ";\n  aid(local);\n")
		case 2:
			module.WriteString("  while (" + expr.Draw(t, "loop") + ") {\n    break;\n  }\n")
		case 3:
			module.WriteString("  for (let i = 0; i < " + expr.Draw(t, "bound") + "; i++) {\n    a += i;\n  }\n")
		case 4:
			module.WriteString("  do {\n    b -= 1;\n  } while (" + expr.Draw(t, "again") + ");\n")
		default:
			module.WriteString("  try {\n    throw new TypeError(String(" + expr.Draw(t, "arg") + "));\n  } catch (e) {\n    if (e instanceof RangeError) return undefined;\n  }\n")
		}
	}
	module.WriteString("  return " + expr.Draw(t, "result") + ";\n}\n")
	return module.String()
}

func TestProperties(t *testing.T) {
	engine := New()

	t.Run("every operator mutant re-parses, differs from the original and splices into a parsable file [verifies SP-1005~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := typescriptFunction(t)
			candidates, err := engine.Candidates(file(source))
			if err != nil {
				t.Fatalf("source %q: %v", source, err)
			}
			for _, c := range candidates {
				if c.Edit.NewSource == c.Original {
					t.Fatalf("mutant equals the original: %q", c.Edit.NewSource)
				}
				spliced, err := splice([]byte(source), c.Edit)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := engine.CallGraph(file(string(spliced))); err != nil {
					t.Fatalf("mutant gives an unparsable file: %v\n%s", err, spliced)
				}
			}
		})
	})

	t.Run("the screen keeps only mutants that parse and differ after normalization [verifies SP-1005~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := typescriptFunction(t)
			candidates, err := engine.Candidates(file(source))
			if err != nil {
				t.Fatal(err)
			}
			kept, _, err := operator.Screen(candidates, engine)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range kept {
				edit := m.EditSet.Edits[0]
				mutated, err := engine.Normalize(typescriptPath, edit.Target, edit.NewSource)
				if err != nil {
					t.Fatalf("kept mutant does not parse: %q", edit.NewSource)
				}
				original, _ := engine.Normalize(typescriptPath, edit.Target, m.Original)
				if mutated == original {
					t.Fatalf("kept mutant equals the original: %q", edit.NewSource)
				}
				spliced, err := splice([]byte(source), edit)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := engine.CallGraph(file(string(spliced))); err != nil {
					t.Fatalf("kept mutant gives an unparsable file: %v\n%s", err, spliced)
				}
			}
		})
	})

	t.Run("sentinels splice into a parsable file [verifies SP-1005~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := typescriptFunction(t)
			sentinels, err := engine.SentinelCandidates(file(source), operator.LocalSeed)
			if err != nil {
				t.Fatal(err)
			}
			if len(sentinels) == 0 {
				t.Fatal("no sentinel candidate")
			}
			for _, s := range sentinels {
				spliced, err := splice([]byte(source), s.Edit)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := engine.CallGraph(file(string(spliced))); err != nil {
					t.Fatalf("sentinel %s gives an unparsable file: %v\n%s", s.Kind, err, spliced)
				}
			}
		})
	})

	t.Run("enumeration and sampling are deterministic for a fixed seed [verifies SP-1005~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := typescriptFunction(t)
			seed := operator.Seed(rapid.Uint64().Draw(t, "seed"))
			run := func() ([]string, []string) {
				candidates, _ := engine.Candidates(file(source))
				kept, _, _ := operator.Screen(candidates, engine)
				sampled := operator.Sample(kept, sampleCap, seed)
				sentinelCandidates, _ := engine.SentinelCandidates(file(source), seed)
				drawn, _ := operator.DrawSentinels(typescriptPath, sentinelCandidates, seed)
				var a, b []string
				for _, m := range sampled {
					a = append(a, m.ID)
				}
				for _, m := range drawn {
					b = append(b, m.ID)
				}
				return a, b
			}
			a1, b1 := run()
			a2, b2 := run()
			if !slices.Equal(a1, a2) || !slices.Equal(b1, b2) {
				t.Fatal("two runs with the same seed differ")
			}
			if len(a1) > sampleCap {
				t.Fatalf("sampled %d above the cap", len(a1))
			}
		})
	})
}
