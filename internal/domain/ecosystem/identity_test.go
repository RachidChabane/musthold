package ecosystem_test

import (
	"testing"

	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
)

func TestTestIdentity(t *testing.T) {
	t.Run("a bound test and a reported test share one identity [verifies SP-64~1]", func(t *testing.T) {
		bound := ecosystem.NewBoundTest("pkg.module", "test_total[1-2]", "tests/test_module.py", 7)
		if bound.ID != ecosystem.NewTestID("pkg.module", "test_total[1-2]") {
			t.Fatalf("id = %q", bound.ID)
		}
		if ecosystem.NewTestID("a", "b") == ecosystem.NewTestID("", "ab") {
			t.Fatal("the classname boundary is lost")
		}
	})
}

func TestMergeOutcome(t *testing.T) {
	order := []ecosystem.TestOutcome{ecosystem.TestSkipped, ecosystem.TestPassed, ecosystem.TestErrored, ecosystem.TestFailed}
	t.Run("the more severe outcome wins in either order [verifies SP-64~1]", func(t *testing.T) {
		for i, low := range order {
			for _, high := range order[i:] {
				if ecosystem.MergeOutcome(low, high) != high || ecosystem.MergeOutcome(high, low) != high {
					t.Fatalf("merge(%s,%s) is not %s", low, high, high)
				}
			}
		}
	})
}

func TestLayoutIsTestFile(t *testing.T) {
	layout := ecosystem.Layout{TestFileGlobs: []string{"test_*.py", "conftest.py", "*_test.go"}}
	cases := map[string]bool{
		"tests/test_cart.py": true, "conftest.py": true, "pkg/cart_test.go": true,
		"cart.py": false, "pkg/cart.go": false, "test.py": false,
	}
	for file, want := range cases {
		t.Run(file+" [verifies SP-62~1]", func(t *testing.T) {
			if got := layout.IsTestFile(file); got != want {
				t.Fatalf("IsTestFile = %v, want %v", got, want)
			}
		})
	}
}

func TestEnclosingDefinition(t *testing.T) {
	definitions := []ecosystem.Definition{
		{Qualname: "Outer.m", Span: ecosystem.Span{Start: 0, End: 100}, StartLine: 3, EndLine: 12},
		{Qualname: "Outer.m.inner", Span: ecosystem.Span{Start: 20, End: 60}, StartLine: 5, EndLine: 8},
		{Qualname: "later", Span: ecosystem.Span{Start: 120, End: 160}, StartLine: 16, EndLine: 18},
	}
	cases := []struct {
		name string
		line int
		want string
		ok   bool
	}{
		{"the innermost definition containing the line", 6, "Outer.m.inner", true},
		{"the enclosing definition outside the nested one", 10, "Outer.m", true},
		{"a decorator line before a definition", 15, "later", true},
		{"a line after every definition", 30, "", false},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-65~1]", func(t *testing.T) {
			got, ok := ecosystem.EnclosingDefinition(definitions, c.line)
			if ok != c.ok || got.Qualname != c.want {
				t.Fatalf("got %q,%v want %q,%v", got.Qualname, ok, c.want, c.ok)
			}
		})
	}
}
