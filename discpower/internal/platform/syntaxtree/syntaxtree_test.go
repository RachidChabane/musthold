package syntaxtree

import (
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

const pairKey = "export function pairKey(a: string, b: string): string {\n  return a < b ? `${a}\x00${b}` : `${b}\x00${a}`;\n}\n"

func TestParse(t *testing.T) {
	typescript := grammars.TypescriptLanguage()
	t.Run("a NUL byte in a template literal parses, and the tree spans the whole file [verifies SP-2427~1]", func(t *testing.T) {
		tree, err := Parse(typescript, []byte(pairKey))
		if err != nil {
			t.Fatal(err)
		}
		defer tree.Release()
		root := tree.RootNode()
		if root.HasError() || int(root.EndByte()) != len(pairKey) {
			t.Fatalf("error %v, end %d of %d", root.HasError(), root.EndByte(), len(pairKey))
		}
	})
	t.Run("the source the caller holds keeps its NUL bytes, and a source without one is read as it is [verifies SP-2427~1]", func(t *testing.T) {
		source := []byte(pairKey)
		read := readable(source)
		if string(source) != pairKey || read[len("export function pairKey(a: string, b: string): string {\n  return a < b ? `${a}")] != nulReading {
			t.Fatalf("source %q, read %q", source, read)
		}
		plain := []byte("const a = 1;\n")
		if got := readable(plain); &got[0] != &plain[0] {
			t.Fatal("a source without a NUL byte was copied")
		}
	})
}
