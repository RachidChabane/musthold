package golang

import (
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
)

const (
	fixtureRoot  = "testdata/parcel"
	fixtureClass = "example.com/parcel"
	internalFile = "parcel_test.go"
	externalFile = "parcel_ext_test.go"
	moduleFile   = "module example.com/parcel\n\ngo 1.22\n"
	libraryFile  = "package parcel\n\nfunc Weigh(items int) int { return items * 3 }\n"
	probePackage = "package p\n\n"
)

func fixture() fs.FS { return os.DirFS(fixtureRoot) }

func bound(file, name string) criterion.BoundTest {
	return criterion.BoundTest{Classname: fixtureClass, Name: name, File: file}
}

func maskFixture(t *testing.T, file, name string) ports.MaskedTest {
	t.Helper()
	masked, err := New().Mask(fixture(), bound(file, name))
	if err != nil {
		t.Fatalf("mask %s: %v", name, err)
	}
	return masked
}

func maskProject(t *testing.T, project fs.FS, file, name string) ports.MaskedTest {
	t.Helper()
	masked, err := New().Mask(project, bound(file, name))
	if err != nil {
		t.Fatalf("mask %s: %v", name, err)
	}
	return masked
}

// moduleProject is an in-memory module with a library package and one test file.
func moduleProject(testSource string) fstest.MapFS {
	return fstest.MapFS{
		"go.mod":     {Data: []byte(moduleFile)},
		"parcel.go":  {Data: []byte(libraryFile)},
		internalFile: {Data: []byte(testSource)},
	}
}

// reporter is what both *testing.T and *rapid.T offer to fail a check.
type reporter interface {
	Helper()
	Fatalf(format string, args ...any)
}

func assertParses(t reporter, unit string) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), "unit.go", probePackage+unit, 0); err != nil {
		t.Fatalf("the unit does not parse: %v\n%s", err, unit)
	}
}

func assertFormatted(t *testing.T, unit string) {
	t.Helper()
	formatted, err := format.Source([]byte(probePackage + unit))
	if err != nil {
		t.Fatalf("format the unit: %v", err)
	}
	if got := probePackage + unit; string(formatted) != got {
		t.Errorf("the unit is not gofmt-clean:\n%s", unit)
	}
}

func assertContains(t *testing.T, text string, wanted ...string) {
	t.Helper()
	for _, fragment := range wanted {
		if !strings.Contains(text, fragment) {
			t.Errorf("missing %q in:\n%s", fragment, text)
		}
	}
}

func assertAbsent(t *testing.T, text string, unwanted ...string) {
	t.Helper()
	for _, fragment := range unwanted {
		if strings.Contains(text, fragment) {
			t.Errorf("unexpected %q in:\n%s", fragment, text)
		}
	}
}
