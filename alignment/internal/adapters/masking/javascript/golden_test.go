package javascript_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const (
	goldenDirectory  = "testdata/golden"
	unitSuffix       = ".unit.ts"
	signaturesSuffix = ".signatures.ts"
	goldenFileMode   = 0o644
)

type goldenCase struct {
	file string
	test criterion.BoundTest
}

func goldenCases() []goldenCase {
	return []goldenCase{
		{"line_total", named(shippingTestFile, tagged("line totals multiply the unit price by the quantity [cites PAR-1~1]"))},
		{"table_rows", named(shippingTestFile, tagged("discounts take 10 percent off 200 [cites PAR-2~1]"))},
		{"nested_hooks", named(shippingTestFile, tagged("discounts limits [cites PAR-2~1] beyond a full price clamp a percentage above one hundred"))},
		{"table_suite", named(shippingTestFile, tagged("shipping to remote costs a number [cites PAR-3~1]"))},
		{"helpers", named(shippingTestFile, "surcharge remote parcels cost more than local ones")},
		{"commonjs", named(ratesTestFile, tagged("heavy parcels pay the fee [cites PAR-2~1]"))},
	}
}

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(goldenDirectory, name)
	if *update {
		if err := os.WriteFile(path, []byte(got), goldenFileMode); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertEqual(t, got, string(want))
}

func TestGoldenMaskedTests(t *testing.T) {
	project := os.DirFS(projectDirectory)
	for _, c := range goldenCases() {
		masked := mustMask(t, project, c.test)
		t.Run("the masked unit of "+c.file+" matches its golden file [verifies SP-1200~1]", func(t *testing.T) {
			compareGolden(t, c.file+unitSuffix, masked.Unit)
		})
		t.Run("the signatures of "+c.file+" match their golden file [verifies SP-1201~1]", func(t *testing.T) {
			compareGolden(t, c.file+signaturesSuffix, masked.Signatures+"\n")
		})
	}
}
