package python_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const (
	goldenDirectory  = "testdata/golden"
	unitSuffix       = ".unit.py"
	signaturesSuffix = ".signatures.py"
	goldenFileMode   = 0o644
)

type goldenCase struct {
	file string
	test criterion.BoundTest
}

func goldenCases() []goldenCase {
	shipping := func(classname, name string) criterion.BoundTest {
		return criterion.BoundTest{Classname: classname, Name: name, File: shippingTestFile}
	}
	return []goldenCase{
		{"line_total", shipping(shippingModule, "test_line_total_multiplies_quantity")},
		{"heavy_parcels", shipping(shippingModule, "test_heavy_parcels_pay_extra[0.1]")},
		{"zones", shipping(shippingModule, "test_europe_costs_more_than_domestic")},
		{"default_card", shipping(shippingModule, "test_quote_uses_default_card")},
		{"class_method", shipping(shippingModule+".TestRates", "test_quote_scales_with_weight")},
		{"class_fixture", shipping(shippingModule+".TestRates", "test_express_quote_is_positive")},
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
		t.Run("the masked unit of "+c.file+" matches its golden file [verifies SP-321~1]", func(t *testing.T) {
			compareGolden(t, c.file+unitSuffix, masked.Unit)
		})
		t.Run("the signatures of "+c.file+" match their golden file [verifies SP-322~1]", func(t *testing.T) {
			compareGolden(t, c.file+signaturesSuffix, masked.Signatures+"\n")
		})
	}
}
