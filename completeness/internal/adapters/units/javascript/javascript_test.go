package javascript_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/completeness/internal/adapters/units/javascript"
	"github.com/RachidChabane/musthold/completeness/internal/domain/code"
	"github.com/RachidChabane/musthold/completeness/internal/domain/hunk"
	"github.com/RachidChabane/musthold/completeness/internal/domain/testlabel"
	"github.com/RachidChabane/musthold/completeness/internal/testkit"
)

// marker is the word that opens a citation; the tests write it as `cites` so that
// none of their lines reads as a requirement tag.
const marker = "verif" + "ies"

func tagged(text string) string { return strings.ReplaceAll(text, "cites", marker) }

func fixture(t *testing.T, side, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(testkit.FixturesDir(), testkit.TypeScriptLedger, side, name))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func index(t *testing.T, file, source string) code.File {
	t.Helper()
	indexed, err := javascript.New().Index(file, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return indexed
}

func qualnames(units []code.Unit) []string {
	names := make([]string, len(units))
	for position, unit := range units {
		names[position] = unit.Qualname
	}
	return names
}

func find(t *testing.T, units []code.Unit, qualname string) code.Unit {
	t.Helper()
	for _, unit := range units {
		if unit.Qualname == qualname {
			return unit
		}
	}
	t.Fatalf("no unit %q among %v", qualname, qualnames(units))
	return code.Unit{}
}

const cart = `import { Tax } from './tax';

export const RATE = 0.2;
const LIMIT: number = 10, FLOOR = 1;
export let counter = 0;

export function total(items: number[]): number {
  return items.length;
}

function* ids() { yield 1; }

export const double = (value: number) => value * 2;
const triple = function (value) { return value * 3; };

export enum Color { Red, Green }
interface Shape { area(): number }
type Id = string;

export class Cart extends Base {
  static created = 0;
  static readonly MAX = 3;
  private items: number[] = [];
  handler = (event: string) => event;
  #secret = 1;

  constructor(owner: string) {
    super();
  }

  get size(): number {
    return this.items.length;
  }

  set size(value: number) {}

  add(item: number): void {
    this.items.push(item);
  }

  static make(): Cart {
    return new Cart('x');
  }
}

export default function main() {}
`

func TestIndexesUnits(t *testing.T) {
	t.Run("a unit is listed per function, method, constructor and constant, and none for a type [verifies SP-1300~1]", func(t *testing.T) {
		got := index(t, "src/cart.ts", cart)
		want := []string{
			"RATE", "LIMIT", "FLOOR", "counter", "total", "ids", "double", "triple", "Color",
			"Cart.constructor", "Cart.size", "Cart.size#2", "Cart.add", "Cart.make",
			"Cart.created", "Cart.MAX", "Cart.handler", "main",
		}
		names := qualnames(got.Units)
		slices.Sort(names)
		slices.Sort(want)
		if !slices.Equal(names, want) {
			t.Fatalf("units\n%v\nwant\n%v", names, want)
		}
	})

	t.Run("a function held by a constant is a function and a value is an assignment [verifies SP-1300~1]", func(t *testing.T) {
		units := index(t, "src/cart.ts", cart).Units
		kinds := map[string]code.Kind{"double": code.KindFunction, "triple": code.KindFunction, "RATE": code.KindAssignment, "Color": code.KindAssignment, "Cart.handler": code.KindFunction, "Cart.MAX": code.KindAssignment, "total": code.KindFunction}
		for name, kind := range kinds {
			if got := find(t, units, name).Kind; got != kind {
				t.Errorf("%s is %s, want %s", name, got, kind)
			}
		}
	})

	t.Run("a constructor answers to the name of its class and a method to its own name [verifies SP-1300~1]", func(t *testing.T) {
		units := index(t, "src/cart.ts", cart).Units
		constructor := find(t, units, "Cart.constructor")
		if constructor.Name != "Cart" || constructor.Owner != "Cart" {
			t.Errorf("constructor %+v", constructor)
		}
		if method := find(t, units, "Cart.add"); method.Name != "add" || method.Owner != "Cart" {
			t.Errorf("method %+v", method)
		}
	})

	t.Run("a unit mentions the names in its code, calls and member names included [verifies SP-1300~1]", func(t *testing.T) {
		units := index(t, "src/totals.ts", string(fixture(t, "change", "src/totals.ts"))).Units
		invoice := find(t, units, "invoiceTotal")
		for _, want := range []string{"lineAmount", "TAX_RATE", "reduce", "map"} {
			if !slices.Contains(invoice.Mentions, want) {
				t.Errorf("invoiceTotal does not mention %q: %v", want, invoice.Mentions)
			}
		}
	})

	t.Run("a declaration of several names gives each its own declarator as its text and one name as its text when alone [verifies SP-1300~1]", func(t *testing.T) {
		units := index(t, "src/cart.ts", cart).Units
		if text := find(t, units, "LIMIT").Text; text != "LIMIT: number = 10" {
			t.Errorf("LIMIT text %q", text)
		}
		if text := find(t, units, "RATE").Text; text != "export const RATE = 0.2;" {
			t.Errorf("RATE text %q", text)
		}
	})

	t.Run("a CommonJS or JSX source is read like a module [verifies SP-1300~1]", func(t *testing.T) {
		got := index(t, "src/rates.js", "const FEE = 3;\nfunction fee(weight) { return FEE * weight; }\nmodule.exports = { fee };\n")
		if names := qualnames(got.Units); !slices.Equal(names, []string{"FEE", "fee"}) {
			t.Fatalf("units %v", names)
		}
		view := index(t, "src/view.jsx", "export const View = () => <div />;\n")
		if names := qualnames(view.Units); !slices.Equal(names, []string{"View"}) {
			t.Fatalf("units %v", names)
		}
		anonymous := index(t, "src/main.ts", "export default function () {}\n")
		if names := qualnames(anonymous.Units); !slices.Equal(names, []string{"default"}) {
			t.Fatalf("units %v", names)
		}
	})

	t.Run("a file the grammars do not serve is refused and a broken file still gives its units [verifies SP-1300~1]", func(t *testing.T) {
		if _, err := javascript.New().Index("a.py", []byte("x = 1")); err == nil {
			t.Fatal("a Python file was indexed")
		}
		broken := index(t, "src/broken.ts", "export function ok() { return 1; }\nexport function oops( {\n")
		if !slices.Contains(qualnames(broken.Units), "ok") {
			t.Fatalf("units %v", qualnames(broken.Units))
		}
	})
}

func TestChangedUnits(t *testing.T) {
	before := index(t, "src/totals.ts", string(fixture(t, "base", "src/totals.ts"))).Units
	after := index(t, "src/totals.ts", string(fixture(t, "change", "src/totals.ts"))).Units

	t.Run("a unit that is new or whose code differs is a hunk and a comment-only change is none [verifies SP-1302~1]", func(t *testing.T) {
		var changed []string
		for _, found := range hunk.Between(before, after) {
			changed = append(changed, found.Unit.Qualname)
		}
		if !slices.Equal(changed, []string{"TAX_RATE", "lineAmount", "invoiceTotal", "applyDiscount"}) {
			t.Fatalf("hunks %v", changed)
		}
	})

	t.Run("a new file makes every unit a hunk [verifies SP-1302~1]", func(t *testing.T) {
		receipts := index(t, "src/receipts.ts", string(fixture(t, "change", "src/receipts.ts"))).Units
		got := hunk.Between(nil, receipts)
		if len(got) != 1 || got[0].Unit.Qualname != "receiptLine" || got[0].Unit.Kind != code.KindFunction || !got[0].New {
			t.Fatalf("hunks %+v", got)
		}
	})

	t.Run("formatting, comments and documentation comments are no change and a literal is [verifies SP-1302~1]", func(t *testing.T) {
		original := index(t, "a.ts", "export function f(x: number) { return x + 1; }").Units
		reformatted := index(t, "a.ts", "/** Adds one. */\nexport function f(\n  x: number // the input\n) {\n  // add\n  return x + 1;\n}\n").Units
		if got := hunk.Between(original, reformatted); len(got) != 0 {
			t.Fatalf("hunks %+v", got)
		}
		literal := index(t, "a.ts", "export function f(x: number) { return x + 2; }").Units
		if got := hunk.Between(original, literal); len(got) != 1 {
			t.Fatalf("hunks %+v", got)
		}
	})
}

func TestServesTestFiles(t *testing.T) {
	t.Run("the files named as tests and those below a test directory hold tests [verifies SP-1301~1]", func(t *testing.T) {
		indexer := javascript.New()
		for _, file := range []string{"cart.test.ts", "src/cart.spec.js", "src/__tests__/cart.ts", "test/support.ts", "packages/a/tests/helpers.js", "src/__mocks__/api.js", "a.test.tsx"} {
			if !indexer.IsTest(file) {
				t.Errorf("%s is not a test file", file)
			}
		}
		for _, file := range []string{"src/cart.ts", "src/testing.ts", "src/contest/entry.js", "src/cart.tsx"} {
			if indexer.IsTest(file) {
				t.Errorf("%s is a test file", file)
			}
		}
	})

	t.Run("the extensions of JavaScript, JSX, TypeScript and TSX are served [verifies SP-1301~1]", func(t *testing.T) {
		indexer := javascript.New()
		for _, file := range []string{"a.js", "a.jsx", "a.mjs", "a.cjs", "a.ts", "a.mts", "a.cts", "a.tsx"} {
			if !indexer.Handles(file) {
				t.Errorf("%s is not served", file)
			}
		}
		for _, file := range []string{"a.py", "a.go", "a.java", "a.kt", "a.json"} {
			if indexer.Handles(file) {
				t.Errorf("%s is served", file)
			}
		}
	})
}

const suiteSource = `import { describe, expect, it, beforeEach } from 'vitest';
import { total, discount } from '../src/cart';

const items = [1, 2];

function fixture() { return items; }

describe('totals', () => {
  let cart;
  beforeEach(() => { cart = fixture(); });

  it('add the items [cites INV-1~1]', () => {
    expect(total(cart)).toBe(3);
  });

  describe('with a discount [cites INV-2~1]', () => {
    it.each([[10], [20]])('takes %s percent off', (percent) => {
      expect(discount(percent)).toBeGreaterThan(0);
    });
  });
});

test.each` + "`" + `
  amount | percent
  ${1}   | ${2}
` + "`" + `('rows $amount and $percent', ({ amount }) => {
  expect(amount).toBe(1);
});

it(` + "`" + `loads ${1 + 1} rows` + "`" + `, async function () {});
it(name, () => {});
`

func TestIndexesTests(t *testing.T) {
	file := index(t, "test/cart.test.ts", tagged(suiteSource))
	var tests []code.Unit
	for _, unit := range file.Units {
		if unit.Kind == code.KindTest {
			tests = append(tests, unit)
		}
	}

	t.Run("every it and test call of any nesting is a test unit named by its titles [verifies SP-1301~1]", func(t *testing.T) {
		if !file.Test {
			t.Error("the file is not a test file")
		}
		want := []string{
			tagged("totals > add the items [cites INV-1~1]"),
			tagged("totals > with a discount [cites INV-2~1] > takes %s percent off"),
			"rows $amount and $percent",
			"loads ${1 + 1} rows",
			"<title>",
		}
		if got := qualnames(tests); !slices.Equal(got, want) {
			t.Fatalf("tests\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("a test mentions what its callback calls and what its suites set up [verifies SP-1301~1]", func(t *testing.T) {
		add := tests[0]
		for _, want := range []string{"total", "expect", "fixture", "cart"} {
			if !slices.Contains(add.Mentions, want) {
				t.Errorf("the test does not mention %q: %v", want, add.Mentions)
			}
		}
		discounted := tests[1]
		if !slices.Contains(discounted.Mentions, "discount") || !slices.Contains(discounted.Mentions, "fixture") {
			t.Errorf("mentions %v", discounted.Mentions)
		}
		if slices.Contains(add.Mentions, "discount") {
			t.Errorf("a sibling suite's code is mentioned: %v", add.Mentions)
		}
	})

	t.Run("a runner's label matches the test whatever the reporter, the tags and the table row [verifies SP-1301~1]", func(t *testing.T) {
		cases := []struct {
			test  int
			label string
		}{
			{0, tagged("totals add the items [cites INV-1~1].totals add the items [cites INV-1~1]")},
			{0, tagged("test/cart.test.ts.totals > add the items [cites INV-1~1]")},
			{1, tagged("totals with a discount [cites INV-2~1] takes 10 percent off")},
			{1, tagged("test/cart.test.ts.totals > with a discount [cites INV-2~1] > takes 20 percent off")},
			{2, "rows 1 and 2"},
			{3, "loads 2 rows"},
		}
		for _, c := range cases {
			if !testlabel.Matches(tests[c.test].Labels, c.label) {
				t.Errorf("test %d does not match %q (%v)", c.test, c.label, tests[c.test].Labels)
			}
		}
		if testlabel.Matches(tests[0].Labels, tagged("totals add the other items [cites INV-1~1]")) {
			t.Error("a label of another test matched")
		}
	})

	t.Run("a title built at run time matches any label of its suites [verifies SP-1301~1]", func(t *testing.T) {
		dynamic := tests[4]
		if !testlabel.Matches(dynamic.Labels, "whatever it is called") {
			t.Errorf("labels %v", dynamic.Labels)
		}
	})

	t.Run("the declarations of the file stay units and the callbacks of a test are not [verifies SP-1301~1]", func(t *testing.T) {
		names := qualnames(file.Units)
		for _, want := range []string{"items", "fixture"} {
			if !slices.Contains(names, want) {
				t.Errorf("no unit %q among %v", want, names)
			}
		}
		for _, unit := range file.Units {
			if unit.Kind == code.KindFunction && unit.Qualname == "cart" {
				t.Errorf("a variable of a suite is a unit: %+v", unit)
			}
		}
	})
}
