package golang

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const (
	goldenDir = "testdata/golden"
	dirMode   = 0o755
	fileMode  = 0o644
)

type goldenCase struct {
	slug string
	file string
	name string
}

var goldenCases = []goldenCase{
	{"table_driven", internalFile, tablePriceTest},
	{"literal_subtest", internalFile, literalTest},
	{"helpers_and_methods", internalFile, helperHeavyTest},
	{"external_package", externalFile, externalTest},
	{"in_module_selector", internalFile, "TestPriceMatchesTable"},
}

func TestGolden(t *testing.T) {
	for _, c := range goldenCases {
		t.Run(c.slug+" [verifies SP-323~1]", func(t *testing.T) {
			masked := maskFixture(t, c.file, c.name)
			compareGolden(t, c.slug+".unit.golden", masked.Unit)
		})
		t.Run(c.slug+" signatures [verifies SP-324~1]", func(t *testing.T) {
			masked := maskFixture(t, c.file, c.name)
			compareGolden(t, c.slug+".signatures.golden", masked.Signatures)
		})
	}
}

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(goldenDir, name)
	if *update {
		if err := os.MkdirAll(goldenDir, dirMode); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), fileMode); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update)", name, err)
	}
	if string(want) != got {
		t.Errorf("%s differs from the golden file:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}
