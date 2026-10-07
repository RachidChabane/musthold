package python_test

import (
	"slices"
	"testing"
	"testing/fstest"
)

const cartModule = `from dataclasses import dataclass

LIMIT = 3


@dataclass
class Cart:
    owner: str
    count: int = 0
    note = "x"

    @property
    def size(self) -> int:
        return self.count

    def add(self, item: str, *, qty: int = 1) -> None:
        """Adds an item."""
        self.count += qty


class Empty:
    pass


def price(item: str,
          qty: int = 1) -> float:  # per unit
    return 1.0


async def fetch(url: str) -> bytes:
    return b""
`

func signatureProject(testSource string) fstest.MapFS {
	return fstest.MapFS{
		sampleTestFile:         {Data: []byte(testSource)},
		"src/shop/__init__.py": {Data: []byte("")},
		"src/shop/cart.py":     {Data: []byte(cartModule)},
		"tests/helpers.py":     {Data: []byte("def make(size: int = 1) -> list:\n    return [size]\n")},
		"legacy.py":            {Data: []byte("def old(value):\n    return value\n")},
	}
}

func signaturesOf(t *testing.T, testSource string) (signatures string, codeFiles []string) {
	t.Helper()
	masked := mustMask(t, signatureProject(testSource), moduleTest("test_check"))
	return masked.Signatures, masked.CodeFiles
}

func TestWritesSignatures(t *testing.T) {
	t.Run("functions and classes are stubbed without decorators, bodies, comments or defaults of attributes [verifies SP-322~1]", func(t *testing.T) {
		got, files := signaturesOf(t, lines(
			"from shop.cart import Cart, Empty, LIMIT, price, fetch",
			"",
			"def test_check():",
			`    Cart("a"); Empty(); LIMIT; price("a"); fetch("u")`,
		))
		want := lines(
			"class Cart:",
			"    owner: str",
			"    count: int",
			"    def size(self) -> int:",
			"        ...",
			"    def add(self, item: str, *, qty: int = 1) -> None:",
			"        ...",
			"",
			"class Empty:",
			"    ...",
			"",
			"def price(item: str,",
			"          qty: int = 1) -> float:",
			"    ...",
			"",
			"async def fetch(url: str) -> bytes:",
			"    ...",
		)
		assertEqual(t, got+"\n", want)
		if !slices.Equal(files, []string{"src/shop/cart.py"}) {
			t.Fatalf("code files %v", files)
		}
	})
	t.Run("a module imported as a whole is stubbed for the names the unit reads from it [verifies SP-322~1]", func(t *testing.T) {
		got, _ := signaturesOf(t, lines(
			"import shop.cart",
			"import shop.cart as sc",
			"from shop import cart",
			"",
			"def test_check():",
			`    shop.cart.price("a"); sc.fetch("u"); cart.price(1)`,
		))
		want := lines(
			"def price(item: str,",
			"          qty: int = 1) -> float:",
			"    ...",
			"",
			"async def fetch(url: str) -> bytes:",
			"    ...",
		)
		assertEqual(t, got+"\n", want)
	})
	t.Run("a relative import resolves from the directory of the test [verifies SP-322~1]", func(t *testing.T) {
		got, files := signaturesOf(t, lines(
			"from .helpers import make",
			"",
			"def test_check():",
			"    make()",
		))
		assertEqual(t, got, "def make(size: int = 1) -> list:\n    ...")
		if !slices.Equal(files, []string{"tests/helpers.py"}) {
			t.Fatalf("code files %v", files)
		}
	})
	t.Run("a module is looked up at the project root and under src [verifies SP-322~1]", func(t *testing.T) {
		got, files := signaturesOf(t, lines(
			"from legacy import old",
			"from shop.cart import price",
			"",
			"def test_check():",
			"    old(price('a'))",
		))
		if !slices.Equal(files, []string{"legacy.py", "src/shop/cart.py"}) {
			t.Fatalf("code files %v, want sorted project files", files)
		}
		assertEqual(t, got, "def old(value):\n    ...\n\ndef price(item: str,\n          qty: int = 1) -> float:\n    ...")
	})
	t.Run("a name that is not a function or a class has no signature [verifies SP-322~1]", func(t *testing.T) {
		got, files := signaturesOf(t, lines(
			"from shop.cart import LIMIT",
			"",
			"def test_check():",
			"    assert LIMIT",
		))
		assertEqual(t, got, "")
		if !slices.Equal(files, []string{"src/shop/cart.py"}) {
			t.Fatalf("code files %v", files)
		}
	})
	t.Run("a library outside the project has no signatures and no code files [verifies SP-322~1]", func(t *testing.T) {
		got, files := signaturesOf(t, lines(
			"import pytest",
			"from dataclasses import dataclass",
			"",
			"def test_check():",
			"    assert pytest and dataclass",
		))
		assertEqual(t, got, "")
		if len(files) != 0 {
			t.Fatalf("code files %v", files)
		}
	})
	t.Run("only the imports the unit keeps are read [verifies SP-322~1]", func(t *testing.T) {
		_, files := signaturesOf(t, lines(
			"from shop.cart import price",
			"from legacy import old",
			"",
			"def test_check():",
			"    price('a')",
		))
		if !slices.Equal(files, []string{"src/shop/cart.py"}) {
			t.Fatalf("code files %v", files)
		}
	})
}
