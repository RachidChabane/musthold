package javascript_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

const cartModule = `export const LIMIT: number = 3;
export const rate = 4;
export let counter = 0;

export type Id = string;

export interface Item {
  id: Id;
  price: number;
}

export enum Color { Red = 1, Green }

export function price(item: Item,
                      qty = 1): number { // per unit
  return qty * item.price;
}

export async function fetchItem(url: string): Promise<Item> {
  return JSON.parse(await (await fetch(url)).text());
}

export const double = (value: number): number => value * 2;
export const triple = function (value: number) { return value * 3; };

export abstract class Shape {
  abstract area(): number;
}

export class Cart extends Shape implements Iterable<Item> {
  private items: Item[] = [];
  protected owner = 'nobody';
  #secret = 1;
  readonly size: number = 0;
  static created = 0;
  handler = (event: string): void => { this.items.length; };

  constructor(owner: string) { super(); }

  add(item: Item, { qty }: { qty?: number } = {}): void {
    this.items.push(item);
  }

  private audit(): void {}

  #hidden(): void {}

  get total(): number { return 1; }

  area(): number { return 0; }
}

export default function main(argv: string[]): void {}

function internal(): void {}
`

func signatureProject(testSource string) fstest.MapFS {
	return fstest.MapFS{
		sampleTestFile:      {Data: []byte(testSource)},
		"src/cart.ts":       {Data: []byte(cartModule)},
		"src/index.ts":      {Data: []byte("export { price as cost, Cart } from './cart';\nexport * from './util';\n")},
		"src/util/index.ts": {Data: []byte("export function clamp(value: number, high: number): number {\n  return Math.min(value, high);\n}\n")},
		"src/legacy.js":     {Data: []byte("const FEE = 3;\nfunction fee(weight) {\n  return FEE * weight;\n}\nmodule.exports = { fee, FEE };\n")},
		"test/support.ts":   {Data: []byte("export function make(size: number): number[] {\n  return [size];\n}\n")},
	}
}

func signaturesOf(t *testing.T, testSource string) (signatures string, codeFiles []string) {
	t.Helper()
	masked := mustMask(t, signatureProject(testSource), sampleTest("check"))
	return masked.Signatures, masked.CodeFiles
}

func TestWritesSignatures(t *testing.T) {
	t.Run("declarations are stubbed without bodies, values, comments or private members [verifies SP-1201~1]", func(t *testing.T) {
		got, files := signaturesOf(t, lines(
			"import main, { Cart, price, fetchItem, double, triple, LIMIT, Item, Color, Id, Shape } from '../src/cart';",
			"",
			"it('check', () => {",
			"  main([]); new Cart('a'); price({} as Item); fetchItem('u'); double(1); triple(1); LIMIT; Color.Red;",
			"  let id: Id; let shape: Shape;",
			"});",
		))
		want := lines(
			"export default function main(argv: string[]): void;",
			"",
			"export class Cart extends Shape implements Iterable<Item> {",
			"  protected owner;",
			"  readonly size: number;",
			"  static created;",
			"  handler = (event: string): void => ...;",
			"  constructor(owner: string);",
			"  add(item: Item, { qty }: { qty?: number } = {}): void;",
			"  get total(): number;",
			"  area(): number;",
			"}",
			"",
			"export function price(item: Item,",
			"                      qty = 1): number;",
			"",
			"export async function fetchItem(url: string): Promise<Item>;",
			"",
			"export const double = (value: number): number => ...;",
			"",
			"export const triple = function (value: number) ...;",
			"",
			"export const LIMIT: number;",
			"",
			"export interface Item {",
			"  id: Id;",
			"  price: number;",
			"}",
			"",
			"export enum Color { Red = 1, Green }",
			"",
			"export type Id = string;",
			"",
			"export abstract class Shape {",
			"  abstract area(): number;",
			"}",
		)
		assertEqual(t, got+"\n", want)
		if !slices.Equal(files, []string{"src/cart.ts"}) {
			t.Errorf("files %v", files)
		}
	})

	t.Run("a name that is not exported is not a signature [verifies SP-1201~1]", func(t *testing.T) {
		got, _ := signaturesOf(t, lines(
			"import { internal } from '../src/cart';",
			"it('check', () => { internal(); });",
		))
		if got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("a re-export, an alias and a barrel of a directory are followed to the declaration [verifies SP-1201~1]", func(t *testing.T) {
		got, files := signaturesOf(t, lines(
			"import { cost, Cart, clamp } from '../src';",
			"it('check', () => { cost(); clamp(1, 2); new Cart('a'); });",
		))
		for _, want := range []string{"export function price(item: Item,", "export class Cart", "export function clamp(value: number, high: number): number;"} {
			if !strings.Contains(got, want) {
				t.Errorf("no %q in:\n%s", want, got)
			}
		}
		if !slices.Equal(files, []string{"src/cart.ts", "src/util/index.ts"}) {
			t.Errorf("files %v", files)
		}
	})

	t.Run("the properties read from a namespace import and from a required module are the signatures [verifies SP-1201~1]", func(t *testing.T) {
		got, files := signaturesOf(t, lines(
			"import * as shop from '../src/cart';",
			"const { fee } = require('../src/legacy');",
			"const legacy = require('../src/legacy');",
			"it('check', () => { shop.price({} as shop.Item); fee(1); legacy.FEE; });",
		))
		for _, want := range []string{"export function price(", "function fee(weight);", "const FEE;"} {
			if !strings.Contains(got, want) {
				t.Errorf("no %q in:\n%s", want, got)
			}
		}
		if strings.Contains(got, "fetchItem") {
			t.Errorf("a property nobody reads was written:\n%s", got)
		}
		if !slices.Equal(files, []string{"src/cart.ts", "src/legacy.js"}) {
			t.Errorf("files %v", files)
		}
	})

	t.Run("a module the project does not hold, or a package, adds nothing [verifies SP-1201~1]", func(t *testing.T) {
		got, files := signaturesOf(t, lines(
			"import { nothing } from '../src/absent';",
			"import { describe } from 'vitest';",
			"it('check', () => { nothing(); describe(); });",
		))
		if got != "" || len(files) != 0 {
			t.Fatalf("signatures %q files %v", got, files)
		}
	})

	t.Run("a module of a test directory is a signature like any other [verifies SP-1201~1]", func(t *testing.T) {
		got, files := signaturesOf(t, lines(
			"import { make } from './support';",
			"it('check', () => { make(1); });",
		))
		if got != "export function make(size: number): number[];" || !slices.Equal(files, []string{"test/support.ts"}) {
			t.Fatalf("signatures %q files %v", got, files)
		}
	})
}
