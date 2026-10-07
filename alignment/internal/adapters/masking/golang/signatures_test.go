package golang

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSignatures(t *testing.T) {
	t.Run("a function is rendered without body or doc [verifies SP-324~1]", func(t *testing.T) {
		signatures := maskFixture(t, internalFile, tablePriceTest).Signatures
		assertContains(t, signatures, "func Price(p Parcel) (int, error)\n")
		assertAbsent(t, signatures, "Validate(); err", "Package parcel", "Price returns")
	})
	t.Run("a type is rendered whole with its exported methods only [verifies SP-324~1]", func(t *testing.T) {
		signatures := maskFixture(t, internalFile, tablePriceTest).Signatures
		assertContains(t, signatures, "type Parcel struct {\n\tID     string\n\tWeight int\n\tZone   Zone\n}")
		assertContains(t, signatures, "func (p *Parcel) Validate() error", "func (p Parcel) Label() string")
		assertAbsent(t, signatures, "reset")
	})
	t.Run("consts and vars are rendered as their spec [verifies SP-324~1]", func(t *testing.T) {
		signatures := maskFixture(t, internalFile, literalTest).Signatures
		assertContains(t, signatures, "const MaxWeight = 30000", `var ErrTooHeavy = errors.New("parcel is too heavy")`)
	})
	t.Run("package-level names of the package under test are found in its code files [verifies SP-324~1]", func(t *testing.T) {
		signatures := maskFixture(t, internalFile, "TestPriceMatchesTable").Signatures
		assertContains(t, signatures, "func base(z Zone) int")
	})
	t.Run("a selector into a package of the module resolves to its directory [verifies SP-324~1]", func(t *testing.T) {
		masked := maskFixture(t, internalFile, "TestPriceMatchesTable")
		assertContains(t, masked.Signatures, "func Default() Table")
		if want := []string{"parcel.go", "rate/rate.go"}; !slices.Equal(masked.CodeFiles, want) {
			t.Errorf("CodeFiles = %v, want %v", masked.CodeFiles, want)
		}
	})
	t.Run("an external test package sees the code through selectors only [verifies SP-324~1]", func(t *testing.T) {
		masked := maskFixture(t, externalFile, externalTest)
		assertContains(t, masked.Signatures, "func Price(p Parcel) (int, error)", "const MaxWeight = 30000")
		assertAbsent(t, masked.Signatures, "func base", "func surcharge")
		if want := []string{"parcel.go"}; !slices.Equal(masked.CodeFiles, want) {
			t.Errorf("CodeFiles = %v, want %v", masked.CodeFiles, want)
		}
	})
	t.Run("a function of a sub-package of the module is rendered [verifies SP-324~1]", func(t *testing.T) {
		masked := maskFixture(t, externalFile, "TestQuoteThroughPublicAPI/quotes_a_national_parcel_[verifies_PRC-14~1]")
		assertContains(t, masked.Signatures, "func Lookup(zone string) (int, bool)")
		if want := []string{"parcel.go", "rate/rate.go"}; !slices.Equal(masked.CodeFiles, want) {
			t.Errorf("CodeFiles = %v, want %v", masked.CodeFiles, want)
		}
	})
	t.Run("signatures are sorted, separated by blank lines and end with a newline [verifies SP-324~1]", func(t *testing.T) {
		signatures := maskFixture(t, internalFile, tablePriceTest).Signatures
		blocks := strings.Split(strings.TrimSuffix(signatures, "\n"), "\n\n")
		if !slices.IsSorted(blocks) {
			t.Errorf("signatures are not sorted:\n%s", signatures)
		}
		if !strings.HasSuffix(signatures, "\n") || strings.HasSuffix(signatures, "\n\n") {
			t.Errorf("signatures do not end with exactly one newline: %q", signatures)
		}
	})
	t.Run("code the unit does not reach is not rendered [verifies SP-324~1]", func(t *testing.T) {
		signatures := maskFixture(t, internalFile, helperHeavyTest).Signatures
		assertAbsent(t, signatures, "ErrNoID", "MaxWeight", "func Price")
	})
	t.Run("a project without go.mod renders package-level names only [verifies SP-324~1]", func(t *testing.T) {
		source := "package parcel\n\nfunc TestWeigh() { _ = Weigh(1) }\n"
		project := fstest.MapFS{
			"parcel.go":  {Data: []byte(libraryFile)},
			internalFile: {Data: []byte(source)},
		}
		masked := maskProject(t, project, internalFile, "TestWeigh")
		if want := "func Weigh(items int) int\n"; masked.Signatures != want {
			t.Errorf("Signatures = %q, want %q", masked.Signatures, want)
		}
	})
	t.Run("a unit that reaches no code has empty signatures and no code files [verifies SP-324~1]", func(t *testing.T) {
		masked := maskProject(t, moduleProject("package parcel\n\nfunc TestNone() {}\n"), internalFile, "TestNone")
		if masked.Signatures != "" || len(masked.CodeFiles) != 0 {
			t.Errorf("Signatures = %q, CodeFiles = %v", masked.Signatures, masked.CodeFiles)
		}
	})
	t.Run("imports outside the module are not rendered [verifies SP-324~1]", func(t *testing.T) {
		masked := maskFixture(t, internalFile, literalTest)
		assertAbsent(t, masked.Signatures, "func Is(", "func New(")
	})
}

func TestModuleResolution(t *testing.T) {
	t.Run("the module path is read from the nearest go.mod [verifies SP-324~1]", func(t *testing.T) {
		project := fstest.MapFS{
			"go.mod":           {Data: []byte("module outer.example/root\n")},
			"nested/go.mod":    {Data: []byte("module \"inner.example/mod\" // nested\n")},
			"nested/pkg/a.go":  {Data: []byte("package pkg\n")},
			"nested/pkg/b.txt": {Data: []byte("")},
		}
		module := findModule(project, "nested/pkg")
		if module.path != "inner.example/mod" || module.dir != "nested" {
			t.Fatalf("module = %+v", module)
		}
		dir, inside := module.directoryOfImport("inner.example/mod/pkg/sub")
		if !inside || dir != "nested/pkg/sub" {
			t.Errorf("directoryOfImport = %q, %v", dir, inside)
		}
		if _, inside := module.directoryOfImport("inner.example/modern"); inside {
			t.Error("a path that only shares a prefix with the module resolved")
		}
		if dir, inside := module.directoryOfImport("inner.example/mod"); !inside || dir != "nested" {
			t.Errorf("the module root resolved to %q, %v", dir, inside)
		}
	})
	t.Run("a tree without go.mod has no module [verifies SP-324~1]", func(t *testing.T) {
		if module := findModule(fstest.MapFS{}, "a/b"); module.path != "" {
			t.Fatalf("module = %+v", module)
		}
	})
	t.Run("an import path stands for its last element without version marks [verifies SP-324~1]", func(t *testing.T) {
		for importPath, want := range map[string]string{
			"strings":                       "strings",
			"github.com/acme/tool/v2":       "tool",
			"gopkg.in/yaml.v3":              "yaml",
			"example.com/parcel/rate":       "rate",
			"github.com/acme/v2fast":        "v2fast",
			"github.com/acme/tool/v2/inner": "inner",
		} {
			if got := packageName(importPath); got != want {
				t.Errorf("packageName(%q) = %q, want %q", importPath, got, want)
			}
		}
	})
}
