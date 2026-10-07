package javascript

import (
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
)

func TestNormalize(t *testing.T) {
	engine := New()
	normalizeAs := func(path string, target mutation.TargetKind, source string) (string, error) {
		return engine.Normalize(path, target, source)
	}
	normalize := func(source string) string {
		t.Helper()
		got, err := normalizeAs(typescriptPath, mutation.TargetDefinition, source)
		if err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		return got
	}

	t.Run("comments, terminators and layout are ignored [verifies SP-1003~1]", func(t *testing.T) {
		plain := normalize("function f(a) { return a+1; }")
		decorated := normalize("function f( a ) {\n  /** Doc. */\n  // why\n  return a + 1 // trailing\n}")
		if plain != decorated {
			t.Fatalf("%q != %q", plain, decorated)
		}
	})

	t.Run("a change of meaning survives normalization [verifies SP-1003~1]", func(t *testing.T) {
		original := normalize("function f(a) { return a + 1; }")
		for _, changed := range []string{
			"function f(a) { return a - 1; }",
			"function f(a) { if (a) { return a + 1; } }",
			"function g(a) { return a + 1; }",
			"function f(a) { return; }",
			"function f(a) { ; }",
		} {
			if normalize(changed) == original {
				t.Errorf("%q normalizes to the original", changed)
			}
		}
	})

	t.Run("a bare method normalizes as a unit [verifies SP-1003~1]", func(t *testing.T) {
		for _, method := range []string{
			"area(): number {\n    return this.w * this.h;\n  }",
			"static async area(): Promise<number> { return 1; }",
			"get area(): number { return 1 }",
			"private area(x: number): number { return x; }",
			"constructor(private w: number) {}",
		} {
			if _, err := normalizeAs(typescriptPath, mutation.TargetDefinition, method); err != nil {
				t.Errorf("%q: %v", method, err)
			}
		}
	})

	t.Run("a mutated bare method differs from its original [verifies SP-1003~1]", func(t *testing.T) {
		if normalize("get area(): number { return this.w * 2 }") == normalize("get area(): number { return this.w / 2 }") {
			t.Fatal("normalized forms are equal")
		}
	})

	t.Run("a declarator normalizes as a unit [verifies SP-1003~1]", func(t *testing.T) {
		for _, declarator := range []string{"FEE = 450", "FEE: number = 450", "TABLE: Map<string, number> = new Map()", "half = (x: number) => x / 2"} {
			if _, err := normalizeAs(typescriptPath, mutation.TargetModuleAssignment, declarator); err != nil {
				t.Errorf("%q: %v", declarator, err)
			}
		}
		first, _ := normalizeAs(typescriptPath, mutation.TargetModuleAssignment, "FEE = 450")
		second, _ := normalizeAs(typescriptPath, mutation.TargetModuleAssignment, "FEE = 451")
		if first == second {
			t.Fatal("two declarators with different values normalize alike")
		}
	})

	t.Run("a unit that does not parse is refused [verifies SP-1003~1]", func(t *testing.T) {
		cases := []struct {
			target mutation.TargetKind
			source string
		}{
			{mutation.TargetDefinition, "function f(:\n  return"},
			{mutation.TargetDefinition, "area(: number { return 1; }"},
			{mutation.TargetModuleAssignment, "FEE = = 450"},
		}
		for _, tt := range cases {
			if _, err := normalizeAs(typescriptPath, tt.target, tt.source); !errors.Is(err, ErrSyntax) {
				t.Errorf("%q: err = %v", tt.source, err)
			}
		}
	})

	t.Run("the grammar is chosen from the file path [verifies SP-1003~1]", func(t *testing.T) {
		typed := "function f(a: number) { return a; }"
		if _, err := normalizeAs("m.ts", mutation.TargetDefinition, typed); err != nil {
			t.Fatal(err)
		}
		if _, err := normalizeAs("m.js", mutation.TargetDefinition, typed); !errors.Is(err, ErrSyntax) {
			t.Fatalf("err = %v", err)
		}
		if _, err := normalizeAs("m.rb", mutation.TargetDefinition, typed); !errors.Is(err, ErrUnsupportedFile) {
			t.Fatalf("err = %v", err)
		}
	})
}
