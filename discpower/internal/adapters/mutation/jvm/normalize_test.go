package jvm

import (
	"errors"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
)

func TestNormalize(t *testing.T) {
	engine := New()
	normalize := func(t *testing.T, path, source string) string {
		t.Helper()
		got, err := engine.Normalize(path, mutation.TargetDefinition, source)
		if err != nil {
			t.Fatalf("Normalize: %v", err)
		}
		return got
	}

	tests := []struct {
		name      string
		path      string
		plain     string
		decorated string
		changed   []string
	}{
		{
			"java", javaPath,
			"int f(int a) {\n    return a+1;\n}",
			"// leading\nint  f( int a )\n{\n    /* why */\n    return a + 1; // trailing\n}",
			[]string{"int f(int a) { return a-1; }", "int f(int a) { if (a > 0) { return a + 1; } return 0; }", "int g(int a) { return a + 1; }", "int f(int a) { return a + 2; }"},
		},
		{
			"java field", javaPath,
			"static final int RATE = 15;",
			"/** Rate. */\nstatic   final int RATE   =   15 ; // per hour",
			[]string{"static final int RATE = 16;", "static final long RATE = 15;"},
		},
		{
			"kotlin", kotlinPath,
			"fun f(a: Int): Int {\n    return a+1\n}",
			"// leading\nfun  f( a : Int ) : Int\n{\n    /* why */\n    return a + 1 // trailing\n}",
			[]string{"fun f(a: Int): Int { return a-1 }", "fun f(a: Int): Int { if (a > 0) { return a + 1 }; return 0 }", "fun g(a: Int): Int { return a + 1 }", "fun f(a: Int): Int { return a + 2 }"},
		},
		{
			"kotlin property", kotlinPath,
			"const val RATE = 15",
			"/** Rate. */\nconst   val RATE   =   15 // per hour",
			[]string{"const val RATE = 16", "val RATE = 15"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+" comments and layout are ignored [verifies SP-1104~1]", func(t *testing.T) {
			if plain, decorated := normalize(t, tt.path, tt.plain), normalize(t, tt.path, tt.decorated); plain != decorated {
				t.Fatalf("%q != %q", plain, decorated)
			}
		})
		t.Run(tt.name+" a change of meaning survives [verifies SP-1104~1]", func(t *testing.T) {
			original := normalize(t, tt.path, tt.plain)
			for _, changed := range tt.changed {
				if normalize(t, tt.path, changed) == original {
					t.Errorf("%q normalizes to the original", changed)
				}
			}
		})
	}

	broken := []struct{ name, path, source string }{
		{"java unbalanced signature", javaPath, "int f( { return 1; }"},
		{"java missing semicolon", javaPath, "int f() { return 1 }"},
		{"java swapped operator", javaPath, "int f(int a) { return a +* 1; }"},
		{"kotlin unbalanced signature", kotlinPath, "fun f(: Int { return 1 }"},
		{"kotlin unbalanced parenthesis", kotlinPath, "fun f(a: Int): Int { return (a + 1 }"},
		{"kotlin stray closing brace", kotlinPath, "fun f(a: Int): Int { return a } }"},
	}
	for _, tt := range broken {
		t.Run(tt.name+" is reported with ErrSyntax [verifies SP-1104~1]", func(t *testing.T) {
			if _, err := engine.Normalize(tt.path, mutation.TargetDefinition, tt.source); !errors.Is(err, ErrSyntax) {
				t.Fatalf("err = %v", err)
			}
		})
	}

	t.Run("an extension without a dialect is refused [verifies SP-1104~1]", func(t *testing.T) {
		if _, err := engine.Normalize("T.scala", mutation.TargetDefinition, "object T"); !errors.Is(err, ErrUnsupportedExtension) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a method of an interface and an override normalize as units [verifies SP-1104~1]", func(t *testing.T) {
		normalize(t, javaPath, "default void d() { }")
		normalize(t, javaPath, "@Override\npublic String toString() { return \"x\"; }")
		normalize(t, kotlinPath, "override fun toString(): String = \"x\"")
		normalize(t, kotlinPath, "private suspend fun f(a: Int): Int { return a }")
	})
}
