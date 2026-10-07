package java_test

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/masking/java"
	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
)

const (
	sampleTestFile   = "src/test/java/shop/SampleTest.java"
	sampleClass      = "shop.SampleTest"
	projectDirectory = "testdata/project"
	pricingTestFile  = "src/test/java/shop/PricingTest.java"
	pricingClass     = "shop.PricingTest"
)

// lines joins source lines and ends the text with a line break.
func lines(source ...string) string { return strings.Join(source, "\n") + "\n" }

// sample is a project holding one test file.
func sample(source string) fstest.MapFS {
	return fstest.MapFS{sampleTestFile: {Data: []byte(tagged(source))}}
}

func sampleTest(classname, name string) criterion.BoundTest {
	return criterion.BoundTest{Classname: classname, Name: name, File: sampleTestFile}
}

func mustMask(t *testing.T, project fs.FS, test criterion.BoundTest) ports.MaskedTest {
	t.Helper()
	masked, err := java.New().Mask(project, test)
	if err != nil {
		t.Fatalf("mask %s: %v", test.Label(), err)
	}
	return masked
}

func maskUnit(t *testing.T, source string, test criterion.BoundTest) string {
	t.Helper()
	return mustMask(t, sample(source), test).Unit
}

func assertEqual(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func criterionOf(file, classname, name string) criterion.BoundTest {
	return criterion.BoundTest{Classname: classname, Name: name, File: file}
}

// marker is the word that opens a citation; the test files write it as `cites` so
// that none of their lines reads as a requirement tag.
const marker = "verif" + "ies"

// tagged spells the citations of a test source or a name written with `cites`.
func tagged(text string) string { return strings.ReplaceAll(text, "cites", marker) }
