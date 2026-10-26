package java_test

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
	unitSuffix       = ".unit.java"
	signaturesSuffix = ".signatures.java"
	goldenFileMode   = 0o644
)

type goldenCase struct {
	file string
	test criterion.BoundTest
}

func goldenCases() []goldenCase {
	pricing := func(classname, name string) criterion.BoundTest {
		return criterion.BoundTest{Classname: classname, Name: name, File: pricingTestFile}
	}
	return []goldenCase{
		{"display_name", pricing(pricingClass, tagged("multiplies the unit price by the quantity [cites PAR-1~1]"))},
		{"method_name", pricing(pricingClass, "rejectsAZeroQuantity()")},
		{"parametrized", pricing(pricingClass, tagged("2 kilos to REMOTE cost more [cites PAR-2~1]"))},
		{"default_invocation", pricing(pricingClass, "anyWeightHasAPrice(int)[2]")},
		{"nested", pricing(pricingClass+"$WithLines", tagged("adds the lines up [cites PAR-1~1]"))},
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
		t.Run("the masked unit of "+c.file+" matches its golden file [verifies SP-1210~1]", func(t *testing.T) {
			compareGolden(t, c.file+unitSuffix, masked.Unit)
		})
		t.Run("the signatures of "+c.file+" match their golden file [verifies SP-1211~1]", func(t *testing.T) {
			compareGolden(t, c.file+signaturesSuffix, masked.Signatures+"\n")
		})
	}
}
