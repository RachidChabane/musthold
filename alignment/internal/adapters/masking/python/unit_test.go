package python_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/musthold/alignment/internal/adapters/masking/python"
	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
)

type unitCase struct {
	name   string
	source string
	test   string
	want   string
}

func runUnitCases(t *testing.T, cases []unitCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-321~1]", func(t *testing.T) {
			assertEqual(t, maskUnit(t, c.source, moduleTest(c.test)), c.want)
		})
	}
}

func TestHandles(t *testing.T) {
	t.Run("only Python files are served [verifies SP-321~1]", func(t *testing.T) {
		masker := python.New()
		for file, want := range map[string]bool{
			"tests/test_shipping.py": true,
			"test_shipping.py":       true,
			"Shipping.java":          false,
			"tests/test_shipping.go": false,
			"README":                 false,
		} {
			if got := masker.Handles(file); got != want {
				t.Errorf("Handles(%q) = %v, want %v", file, got, want)
			}
		}
	})
}

func TestLocatesTheBoundTest(t *testing.T) {
	const source = "def test_same():\n    assert 1\n\n\nclass TestA:\n    def test_same(self):\n        assert 2\n\n    def test_only(self):\n        assert 3\n"
	t.Run("a classname that ends with a class selects its method [verifies SP-321~1]", func(t *testing.T) {
		assertEqual(t, maskUnit(t, source, classTest("TestA", "test_same")), "def test_case(self):\n    assert 2\n")
	})
	t.Run("a module classname selects the module-level function [verifies SP-321~1]", func(t *testing.T) {
		assertEqual(t, maskUnit(t, source, moduleTest("test_same")), "def test_case():\n    assert 1\n")
	})
	t.Run("a method is found in any class when the classname names none [verifies SP-321~1]", func(t *testing.T) {
		assertEqual(t, maskUnit(t, source, classTest("TestGone", "test_only")), "def test_case(self):\n    assert 3\n")
	})
	t.Run("the parametrization suffix of a pytest name is ignored [verifies SP-321~1]", func(t *testing.T) {
		assertEqual(t, maskUnit(t, source, moduleTest("test_same[1-a]")), "def test_case():\n    assert 1\n")
	})
	t.Run("the files of the masked test are the test file only [verifies SP-321~1]", func(t *testing.T) {
		masked := mustMask(t, sample(source), moduleTest("test_same"))
		if len(masked.TestFiles) != 1 || masked.TestFiles[0] != sampleTestFile {
			t.Fatalf("test files %v", masked.TestFiles)
		}
	})
}

func TestRefusesWhatItCannotMask(t *testing.T) {
	t.Run("a test the file does not declare is not found [verifies SP-321~1]", func(t *testing.T) {
		_, err := python.New().Mask(sample("def test_other():\n    pass\n"), moduleTest("test_missing"))
		if !errors.Is(err, ports.ErrTestNotFound) {
			t.Fatalf("got %v, want ErrTestNotFound", err)
		}
	})
	t.Run("a helper with the name of the test is not a test [verifies SP-321~1]", func(t *testing.T) {
		_, err := python.New().Mask(sample("HELPER = 1\n"), moduleTest("HELPER"))
		if !errors.Is(err, ports.ErrTestNotFound) {
			t.Fatalf("got %v, want ErrTestNotFound", err)
		}
	})
	t.Run("a missing test file is an error [verifies SP-321~1]", func(t *testing.T) {
		if _, err := python.New().Mask(fstest.MapFS{}, moduleTest("test_any")); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("a test whose code spells its own name is refused as a leak [verifies SP-321~1]", func(t *testing.T) {
		source := lines(
			"from shop import total_price_discount",
			"",
			"def test_total_price_discount():",
			"    assert total_price_discount(1)",
		)
		_, err := python.New().Mask(sample(source), moduleTest("test_total_price_discount"))
		if !errors.Is(err, masking.ErrLeak) {
			t.Fatalf("got %v, want ErrLeak", err)
		}
	})
	t.Run("a file with a syntax error does not panic [verifies SP-321~1]", func(t *testing.T) {
		source := lines("def test_broken():", "    assert compute(", "", "def test_after(:", "    pass")
		for _, name := range []string{"test_broken", "test_after"} {
			_, _ = python.New().Mask(sample(source), moduleTest(name))
		}
	})
}

func TestReachesHelpers(t *testing.T) {
	runUnitCases(t, []unitCase{
		{
			name: "helpers are reached transitively and listed in source order",
			source: lines(
				"from shop import price",
				"",
				"TAX = 1.2",
				"",
				"def unused():",
				"    return 0",
				"",
				"def base(x):",
				"    return price(x) * TAX",
				"",
				"def total(x):",
				"    return base(x) + 1",
				"",
				"def test_check():",
				"    assert total(2) > 0",
			),
			test: "test_check",
			want: lines(
				"from shop import price",
				"",
				"helper_1 = 1.2",
				"",
				"def helper_2(x):",
				"    return price(x) * helper_1",
				"",
				"def helper_3(x):",
				"    return helper_2(x) + 1",
				"",
				"def test_case():",
				"    assert helper_3(2) > 0",
			),
		},
		{
			name: "annotated assignments are helpers",
			source: lines(
				"LIMIT: int = 3",
				"",
				"def test_check():",
				"    assert LIMIT",
			),
			test: "test_check",
			want: lines("helper_1: int = 3", "", "def test_case():", "    assert helper_1"),
		},
		{
			name: "a fixture is reached through the name of a parameter",
			source: lines(
				"import pytest",
				"",
				"@pytest.fixture",
				"def basket():",
				"    return [1]",
				"",
				"def test_check(basket):",
				"    assert basket",
			),
			test: "test_check",
			want: lines(
				"import pytest",
				"",
				"@pytest.fixture",
				"def helper_1():",
				"    return [1]",
				"",
				"def test_case(helper_1):",
				"    assert helper_1",
			),
		},
		{
			name: "attribute names and keyword names are not references",
			source: lines(
				"def render():",
				"    return 1",
				"",
				"def test_check(obj):",
				"    assert obj.render(render=2)",
			),
			test: "test_check",
			want: lines("def test_case(obj):", "    assert obj.render(render=2)"),
		},
		{
			name: "a class used by the test is a helper",
			source: lines(
				"class Box:",
				"    def open(self):",
				"        return Box",
				"",
				"def test_check():",
				"    assert Box().open()",
			),
			test: "test_check",
			want: lines(
				"class helper_1:",
				"    def open(self):",
				"        return helper_1",
				"",
				"def test_case():",
				"    assert helper_1().open()",
			),
		},
	})
}

func TestKeepsTheImportsTheUnitUses(t *testing.T) {
	runUnitCases(t, []unitCase{
		{
			name: "only the used aliases are rebuilt, imports first",
			source: lines(
				"import os",
				"import os.path",
				"import json as js",
				"import sys as system, re",
				"from . import sibling",
				"from ..pkg.mod import a as b, c, d",
				"from typing import List",
				"",
				"def test_check():",
				"    os.getcwd()",
				"    js.dumps(1)",
				"    system.exit",
				"    sibling.x",
				"    b(c)",
			),
			test: "test_check",
			want: lines(
				"import os",
				"import os.path",
				"import json as js",
				"import sys as system",
				"from . import sibling",
				"from ..pkg.mod import a as b, c",
				"",
				"def test_case():",
				"    os.getcwd()",
				"    js.dumps(1)",
				"    system.exit",
				"    sibling.x",
				"    b(c)",
			),
		},
		{
			name: "an import used only by a removed marker is dropped",
			source: lines(
				"import pytest",
				"",
				`@pytest.mark.verifies("X-1~1")`,
				"def test_check():",
				"    assert 1",
			),
			test: "test_check",
			want: lines("def test_case():", "    assert 1"),
		},
		{
			name: "a name read as an attribute does not use the import of that name",
			source: lines(
				"import json",
				"",
				"def test_check(obj):",
				"    assert obj.json",
			),
			test: "test_check",
			want: lines("def test_case(obj):", "    assert obj.json"),
		},
	})
}

func TestRemovesWhatNamesTheCriterion(t *testing.T) {
	runUnitCases(t, []unitCase{
		{
			name: "comments are removed, whole lines and trailing ones",
			source: lines(
				"def helper():",
				"    return 1  # trailing",
				"",
				"# standalone",
				"def test_check():",
				"    # inner",
				"    assert helper()  # last",
			),
			test: "test_check",
			want: lines(
				"def helper_1():",
				"    return 1",
				"",
				"def test_case():",
				"    assert helper_1()",
			),
		},
		{
			name: "docstrings of functions and classes are removed",
			source: lines(
				"class Box:",
				`    """Holds things."""`,
				"",
				"    def open(self):",
				`        """Opens."""`,
				"        return 1",
				"",
				"def test_check():",
				`    """Checks."""`,
				"    assert Box().open()",
			),
			test: "test_check",
			want: lines(
				"class helper_1:",
				"",
				"    def open(self):",
				"        return 1",
				"",
				"def test_case():",
				"    assert helper_1().open()",
			),
		},
		{
			name: "a body that held only a docstring keeps a placeholder",
			source: lines(
				"class Base:",
				`    """Only a docstring."""`,
				"",
				"def test_check():",
				"    Base()",
			),
			test: "test_check",
			want: lines("class helper_1:", "    ...", "", "def test_case():", "    helper_1()"),
		},
		{
			name: "verifies decorators are removed whatever their qualifier",
			source: lines(
				"import pytest",
				"from marks import verifies",
				"import marks as mk",
				"",
				`@pytest.mark.verifies("X-1~1")`,
				`@verifies("X-2~1")`,
				`@mk.verifies("X-3~1")`,
				"def test_check():",
				"    assert 1",
			),
			test: "test_check",
			want: lines("def test_case():", "    assert 1"),
		},
		{
			name: "other decorators are kept",
			source: lines(
				"import pytest",
				"",
				`@pytest.mark.verifies("X-1~1")`,
				"@pytest.mark.slow",
				`@pytest.mark.parametrize("n", [1, 2])`,
				"def test_check(n):",
				"    assert n",
			),
			test: "test_check",
			want: lines(
				"import pytest",
				"",
				"@pytest.mark.slow",
				`@pytest.mark.parametrize("n", [1, 2])`,
				"def test_case(n):",
				"    assert n",
			),
		},
		{
			name: "a verifies decorator between two others leaves them intact",
			source: lines(
				"import pytest",
				"",
				"@pytest.mark.slow",
				`@pytest.mark.verifies("X-1~1")  # cited`,
				"@pytest.mark.flaky",
				"def test_check():",
				"    assert 1",
			),
			test: "test_check",
			want: lines(
				"import pytest",
				"",
				"@pytest.mark.slow",
				"@pytest.mark.flaky",
				"def test_case():",
				"    assert 1",
			),
		},
	})
}

func TestUsesNeutralNames(t *testing.T) {
	runUnitCases(t, []unitCase{
		{
			name: "the test is test_case and helpers are numbered in source order",
			source: lines(
				"def second():",
				"    return first()",
				"",
				"def first():",
				"    return 1",
				"",
				"def test_check():",
				"    assert second()",
			),
			test: "test_check",
			want: lines(
				"def helper_1():",
				"    return helper_2()",
				"",
				"def helper_2():",
				"    return 1",
				"",
				"def test_case():",
				"    assert helper_1()",
			),
		},
		{
			name: "attribute fields and keyword names keep their spelling",
			source: lines(
				"def total(x):",
				"    return x",
				"",
				"def test_check(mod):",
				"    assert mod.total(total=total(1))",
			),
			test: "test_check",
			want: lines(
				"def helper_1(x):",
				"    return x",
				"",
				"def test_case(mod):",
				"    assert mod.total(total=helper_1(1))",
			),
		},
		{
			name: "string literals are not renamed",
			source: lines(
				"def total():",
				"    return 'total'",
				"",
				"def test_check():",
				"    assert total() == 'total'",
			),
			test: "test_check",
			want: lines(
				"def helper_1():",
				"    return 'total'",
				"",
				"def test_case():",
				"    assert helper_1() == 'total'",
			),
		},
	})
}

func TestMasksAMethodAsAUnit(t *testing.T) {
	const source = `import pytest
from shop import Cart

TAX = 2

def build():
    return Cart()

class TestCart:
    """Cart tests."""

    def setup(self):
        return build()

    def fill(self, cart):
        return self.setup()

    def other(self):
        return TAX

    @pytest.fixture
    def card(self):
        return 1

    @pytest.mark.verifies("C-1~1")
    def test_fill(self, card):
        """Fills."""
        assert self.fill(card)
`
	t.Run("the method is dedented with its siblings and module helpers, without the class [verifies SP-321~1]", func(t *testing.T) {
		want := lines(
			"import pytest",
			"from shop import Cart",
			"",
			"def helper_1():",
			"    return Cart()",
			"",
			"def helper_2(self):",
			"    return helper_1()",
			"",
			"def helper_3(self, cart):",
			"    return self.helper_2()",
			"",
			"@pytest.fixture",
			"def helper_4(self):",
			"    return 1",
			"",
			"def test_case(self, helper_4):",
			"    assert self.helper_3(helper_4)",
		)
		got := maskUnit(t, source, classTest("TestCart", "test_fill"))
		assertEqual(t, got, want)
	})
	t.Run("the class and the siblings the test does not reach stay out [verifies SP-321~1]", func(t *testing.T) {
		got := maskUnit(t, source, classTest("TestCart", "test_fill"))
		for _, absent := range []string{"TestCart", "TAX", "other", "class "} {
			if strings.Contains(got, absent) {
				t.Errorf("%q is in the unit:\n%s", absent, got)
			}
		}
	})
}

func TestNormalisesTheLayout(t *testing.T) {
	t.Run("trailing blanks go, blank runs collapse, units are one blank line apart [verifies SP-321~1]", func(t *testing.T) {
		source := "def helper():   \n    x = 1   \n\n\n\n    return x\n\n\n\n\ndef test_check():\n    assert helper()\n\n\n"
		want := "def helper_1():\n    x = 1\n\n    return x\n\ndef test_case():\n    assert helper_1()\n"
		assertEqual(t, maskUnit(t, source, moduleTest("test_check")), want)
	})
	t.Run("the unit ends with a single line break [verifies SP-321~1]", func(t *testing.T) {
		got := maskUnit(t, "def test_check():\n    pass", moduleTest("test_check"))
		assertEqual(t, got, "def test_case():\n    pass\n")
	})
	t.Run("masking is deterministic [verifies SP-321~1]", func(t *testing.T) {
		project := sample("import os\n\ndef a():\n    return os\n\ndef test_check():\n    assert a()\n")
		first := mustMask(t, project, moduleTest("test_check"))
		second := mustMask(t, project, moduleTest("test_check"))
		if first.Unit != second.Unit || first.Signatures != second.Signatures {
			t.Fatal("two runs differ")
		}
	})
}
