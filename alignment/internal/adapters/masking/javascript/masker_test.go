package javascript_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/musthold/alignment/internal/adapters/masking/javascript"
	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
)

func TestServesJavaScriptAndTypeScriptFiles(t *testing.T) {
	t.Run("the masker serves the extensions of JavaScript and TypeScript and refuses the others [verifies SP-1205~1]", func(t *testing.T) {
		masker := javascript.New()
		for _, file := range []string{"a.test.js", "a.test.jsx", "a.test.mjs", "a.test.cjs", "a.test.ts", "a.test.mts", "a.test.tsx"} {
			if !masker.Handles(file) {
				t.Errorf("%s is not served", file)
			}
		}
		for _, file := range []string{"a_test.py", "a_test.go", "A.java", "A.kt", "a.md"} {
			if masker.Handles(file) {
				t.Errorf("%s is served", file)
			}
		}
		_, err := masker.Mask(sample("x"), criterion.BoundTest{Name: "x", File: "a_test.go"})
		if !errors.Is(err, ports.ErrUnsupportedFile) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestMasksATest(t *testing.T) {
	t.Run("the titles, the comments and the citation tags are gone and the helpers have neutral names [verifies SP-1200~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"import { describe, expect, it } from 'vitest';",
			"import { price } from '../src/price';",
			"",
			"const UNIT = 4; // per unit",
			"",
			"/* the shop sells by the piece */",
			"function double(value: number) {",
			"  return value * 2;",
			"}",
			"",
			tagged("describe('pricing [cites PAR-9~1]', () => {"),
			"  // no discount here",
			tagged("  it('doubles a unit [cites PAR-9~1]', () => {"),
			"    expect(price(double(UNIT))).toBe(8);",
			"  });",
			"});",
		), sampleTest(tagged("pricing doubles a unit [cites PAR-9~1]")))
		assertEqual(t, got, lines(
			"import { describe, expect, it } from 'vitest';",
			"import { price } from '../src/price';",
			"",
			"const helper1 = 4;",
			"",
			"function helper2(value: number) {",
			"  return value * 2;",
			"}",
			"",
			"describe('case', () => {",
			"  it('case', () => {",
			"    expect(price(helper2(helper1))).toBe(8);",
			"  });",
			"});",
		))
	})

	t.Run("a declaration the test does not reach, a sibling test and an unused import are left out [verifies SP-1200~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"import { a } from './a';",
			"import { b } from './b';",
			"",
			"const UNUSED = 1;",
			"const SHARED = 2;",
			"",
			"describe('group', () => {",
			"  it('first', () => {",
			"    expect(a(SHARED)).toBe(1);",
			"  });",
			"  it('second', () => {",
			"    expect(b(UNUSED)).toBe(1);",
			"  });",
			"});",
		), sampleTest("group first"))
		for _, absent := range []string{"UNUSED", "helper2", "./b", "second"} {
			if strings.Contains(got, absent) {
				t.Errorf("the unit holds %q:\n%s", absent, got)
			}
		}
		if !strings.Contains(got, "const helper1 = 2;") || !strings.Contains(got, "./a") {
			t.Errorf("the unit lost what the test reaches:\n%s", got)
		}
	})

	t.Run("the hooks and the setup calls of the enclosing suites stay, with the variables they assign [verifies SP-1200~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"import { Cart } from '../src/cart';",
			"",
			"vi.mock('../src/audit');",
			"",
			"describe('cart', () => {",
			"  let cart: Cart;",
			"  let other: Cart;",
			"  beforeEach(() => {",
			"    cart = new Cart();",
			"  });",
			"  describe('with items', () => {",
			"    afterEach(() => vi.restoreAllMocks());",
			"    it('counts them', () => {",
			"      cart.add(1);",
			"      expect(cart.size).toBe(1);",
			"    });",
			"  });",
			"  describe('empty', () => {",
			"    it('has no items', () => {",
			"      expect(other.size).toBe(0);",
			"    });",
			"  });",
			"});",
		), sampleTest("cart with items counts them"))
		assertEqual(t, got, lines(
			"import { Cart } from '../src/cart';",
			"",
			"vi.mock('../src/audit');",
			"",
			"describe('case', () => {",
			"  let helper1: Cart;",
			"",
			"  beforeEach(() => {",
			"    helper1 = new Cart();",
			"  });",
			"",
			"  describe('case', () => {",
			"    afterEach(() => vi.restoreAllMocks());",
			"",
			"    it('case', () => {",
			"      helper1.add(1);",
			"      expect(helper1.size).toBe(1);",
			"    });",
			"  });",
			"});",
		))
	})

	t.Run("a shorthand property keeps its key under a renamed variable [verifies SP-1200~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"const fixture = { size: 2 };",
			"",
			"it('reads the fixture', () => {",
			"  const wrapped = { fixture };",
			"  expect(wrapped.fixture.size).toBe(2);",
			"});",
		), sampleTest("reads the fixture"))
		if !strings.Contains(got, "{ fixture: helper1 }") || !strings.Contains(got, "wrapped.fixture.size") {
			t.Fatalf("got:\n%s", got)
		}
	})

	t.Run("a test with a template literal title and an async function is masked like any other [verifies SP-1200~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"test(`loads ${1 + 1} rows`, async function () {",
			"  expect(await load()).toBe(2);",
			"});",
		), sampleTest("loads 2 rows"))
		assertEqual(t, got, lines(
			"test('case', async function () {",
			"  expect(await load()).toBe(2);",
			"});",
		))
	})
}

func TestMasksATsxTest(t *testing.T) {
	t.Run("a component a JSX element names is reached like a function a call names [verifies SP-1200~1]", func(t *testing.T) {
		project := fstest.MapFS{"test/badge.test.tsx": {Data: []byte(lines(
			"import { render } from '@testing-library/react';",
			"",
			"const Badge = ({ label }: { label: string }) => <span>{label}</span>;",
			"const Unused = () => <i />;",
			"",
			"it('renders a badge', () => {",
			"  render(<Badge label=\"new\" />);",
			"});",
		))}}
		got := mustMask(t, project, named("test/badge.test.tsx", "renders a badge")).Unit
		assertEqual(t, got, lines(
			"import { render } from '@testing-library/react';",
			"",
			"const Helper1 = ({ label }: { label: string }) => <span>{label}</span>;",
			"",
			"it('case', () => {",
			"  render(<Helper1 label=\"new\" />);",
			"});",
		))
	})
}

func TestMasksATableDrivenTest(t *testing.T) {
	const source = `import { total } from './total';

describe('totals', () => {
  it.each([
    [10, 200, 180],
    [50, 80, 40],
  ])('take %s percent off %s [cites PAR-2~1]', (percent, amount, expected) => {
    expect(total(amount, percent)).toBe(expected);
  });

  it.each` + "`" + `
    amount | percent
    ${1}   | ${2}
  ` + "`" + `('rows $amount and $percent', ({ amount, percent }) => {
    expect(total(amount, percent)).toBeGreaterThan(0);
  });
});
`

	t.Run("the table and the parameters stay and the title is neutral [verifies SP-1203~1]", func(t *testing.T) {
		got := maskUnit(t, source, sampleTest(tagged("totals take 10 percent off 200 [cites PAR-2~1]")))
		assertEqual(t, got, lines(
			"import { total } from './total';",
			"",
			"describe('case', () => {",
			"  it.each([",
			"    [10, 200, 180],",
			"    [50, 80, 40],",
			"  ])('case', (percent, amount, expected) => {",
			"    expect(total(amount, percent)).toBe(expected);",
			"  });",
			"});",
		))
	})

	t.Run("a tagged template table is read the same way [verifies SP-1203~1]", func(t *testing.T) {
		got := maskUnit(t, source, sampleTest("totals rows 1 and 2"))
		if !strings.Contains(got, "it.each`") || !strings.Contains(got, "('case', ({ amount, percent }) => {") {
			t.Fatalf("got:\n%s", got)
		}
	})
}

func TestFindsTheBoundTest(t *testing.T) {
	const source = `describe('discounts', () => {
  it.each([[1], [2]])('take %s off', (n) => {
    expect(n).toBeGreaterThan(0);
  });

  it('take 3 off', () => {
    expect(3).toBe(3);
  });

  describe('limits [cites PAR-2~1]', () => {
    it('clamp a percentage', () => {
      expect(clamp(150)).toBe(100);
    });
  });
});
`

	t.Run("Jest names a test by its titles joined with spaces, a tag of a suite included [verifies SP-1202~1]", func(t *testing.T) {
		got := maskUnit(t, source, sampleTest(tagged("discounts limits [cites PAR-2~1] clamp a percentage")))
		if !strings.Contains(got, "clamp(150)") {
			t.Fatalf("got:\n%s", got)
		}
	})

	t.Run("Vitest names a test by its titles joined with the separator of its reporter [verifies SP-1202~1]", func(t *testing.T) {
		test := criterion.BoundTest{Classname: sampleTestFile, Name: tagged("discounts > limits [cites PAR-2~1] > clamp a percentage"), File: sampleTestFile}
		if got := mustMask(t, sample(source), test).Unit; !strings.Contains(got, "clamp(150)") {
			t.Fatalf("got:\n%s", got)
		}
	})

	t.Run("the values of a table row stand for the placeholders of the title [verifies SP-1202~1]", func(t *testing.T) {
		got := maskUnit(t, source, sampleTest("discounts take 2 off"))
		if !strings.Contains(got, "expect(n).toBeGreaterThan(0)") {
			t.Fatalf("got:\n%s", got)
		}
	})

	t.Run("a literal title wins over a title that holds placeholders [verifies SP-1202~1]", func(t *testing.T) {
		got := maskUnit(t, source, sampleTest("discounts take 3 off"))
		if !strings.Contains(got, "expect(3).toBe(3)") {
			t.Fatalf("got:\n%s", got)
		}
	})

	t.Run("two tests that share a title are told apart by the line of their report [verifies SP-1202~1]", func(t *testing.T) {
		twins := lines(
			"describe('a', () => {",
			"  it('same', () => {",
			"    expect(1).toBe(1);",
			"  });",
			"});",
			"describe('a', () => {",
			"  it('same', () => {",
			"    expect(2).toBe(2);",
			"  });",
			"});",
		)
		first := criterion.BoundTest{Name: "a same", File: sampleTestFile, Line: 2}
		second := criterion.BoundTest{Name: "a same", File: sampleTestFile, Line: 7}
		if got := mustMask(t, sample(twins), first).Unit; !strings.Contains(got, "expect(1)") {
			t.Errorf("first: %s", got)
		}
		if got := mustMask(t, sample(twins), second).Unit; !strings.Contains(got, "expect(2)") {
			t.Errorf("second: %s", got)
		}
	})

	t.Run("a test the file does not declare is not found [verifies SP-1202~1]", func(t *testing.T) {
		_, err := javascript.New().Mask(sample(source), sampleTest("discounts nothing like it"))
		if !errors.Is(err, ports.ErrTestNotFound) {
			t.Fatalf("error = %v", err)
		}
		_, err = javascript.New().Mask(sample(source), criterion.BoundTest{Name: "x", File: "test/missing.test.ts"})
		if err == nil || errors.Is(err, ports.ErrTestNotFound) {
			t.Fatalf("a missing file gave %v", err)
		}
	})
}

func TestRefusesATestThatSpellsItsTitle(t *testing.T) {
	t.Run("a string in the body that spells the title makes the masked input leak [verifies SP-1204~1]", func(t *testing.T) {
		_, err := javascript.New().Mask(sample(lines(
			"it('grace period is free', () => {",
			"  const label = 'grace period is free';",
			"  expect(label).toBeTruthy();",
			"});",
		)), sampleTest("grace period is free"))
		if !errors.Is(err, masking.ErrLeak) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("the marker word anywhere in the body makes it leak [verifies SP-1204~1]", func(t *testing.T) {
		_, err := javascript.New().Mask(sample(lines(
			"it('is fine', () => {",
			"  expect(verifies()).toBe(1);",
			"});",
		)), sampleTest("is fine"))
		if !errors.Is(err, masking.ErrLeak) {
			t.Fatalf("error = %v", err)
		}
	})
}
