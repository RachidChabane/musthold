package python_test

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/masking/python"
	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
)

const (
	sampleTestFile   = "tests/test_sample.py"
	sampleModule     = "tests.test_sample"
	projectDirectory = "testdata/project"
	shippingTestFile = "tests/test_shipping.py"
	shippingModule   = "tests.test_shipping"
)

// lines joins source lines and ends the text with a line break.
func lines(source ...string) string { return strings.Join(source, "\n") + "\n" }

// sample is a project holding one test file.
func sample(source string) fstest.MapFS {
	return fstest.MapFS{sampleTestFile: {Data: []byte(source)}}
}

func moduleTest(name string) criterion.BoundTest {
	return criterion.BoundTest{Classname: sampleModule, Name: name, File: sampleTestFile}
}

func classTest(class, name string) criterion.BoundTest {
	return criterion.BoundTest{Classname: sampleModule + "." + class, Name: name, File: sampleTestFile}
}

func mustMask(t *testing.T, project fs.FS, test criterion.BoundTest) ports.MaskedTest {
	t.Helper()
	masked, err := python.New().Mask(project, test)
	if err != nil {
		t.Fatalf("mask %s: %v", test.Label(), err)
	}
	return masked
}

// maskUnit masks a test of a one-file project and returns its unit.
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
