package golang

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
)

var (
	tablePriceTest  = "TestPrice/an_international_parcel_" + citeReported("PRC-12~1")
	literalTest     = "TestValidate/rejects_a_parcel_above_the_limit_" + citeReported("PRC-13~1")
	uncitedTest     = "TestValidate/rejects_a_parcel_without_identifier"
	helperHeavyTest = "TestLabel"
	externalTest    = "TestQuoteThroughPublicAPI/refuses_an_oversized_parcel_" + citeReported("PRC-14~1")
)

func TestHandles(t *testing.T) {
	masker := New()
	t.Run("serves Go test files and nothing else [verifies SP-323~1]", func(t *testing.T) {
		for file, want := range map[string]bool{
			"parcel_test.go":     true,
			"pkg/deep/a_test.go": true,
			"parcel.go":          false,
			"test_parcel.py":     false,
			"parcel_test.go.txt": false,
		} {
			if got := masker.Handles(file); got != want {
				t.Errorf("Handles(%q) = %v, want %v", file, got, want)
			}
		}
	})
}

func TestMaskErrors(t *testing.T) {
	t.Run("a file no Go masker serves is unsupported [verifies SP-323~1]", func(t *testing.T) {
		_, err := New().Mask(fixture(), bound("test_parcel.py", "TestPrice"))
		if !errors.Is(err, ports.ErrUnsupportedFile) {
			t.Fatalf("error = %v, want ErrUnsupportedFile", err)
		}
	})
	t.Run("an undeclared test is not found [verifies SP-323~1]", func(t *testing.T) {
		_, err := New().Mask(fixture(), bound(internalFile, "TestMissing/anything"))
		if !errors.Is(err, ports.ErrTestNotFound) {
			t.Fatalf("error = %v, want ErrTestNotFound", err)
		}
	})
	t.Run("a test declared in another file is not found [verifies SP-323~1]", func(t *testing.T) {
		_, err := New().Mask(fixture(), bound(internalFile, "TestQuoteThroughPublicAPI"))
		if !errors.Is(err, ports.ErrTestNotFound) {
			t.Fatalf("error = %v, want ErrTestNotFound", err)
		}
	})
	t.Run("a missing file is an error, not a panic [verifies SP-323~1]", func(t *testing.T) {
		if _, err := New().Mask(fixture(), bound("absent_test.go", "TestPrice")); err == nil {
			t.Fatal("expected an error")
		}
	})
	t.Run("a file that does not parse is an error [verifies SP-323~1]", func(t *testing.T) {
		project := moduleProject("package parcel\n\nfunc TestBroken(t *testing.T) {\n")
		if _, err := New().Mask(project, bound(internalFile, "TestBroken")); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestUnitSelection(t *testing.T) {
	t.Run("a literal subtest keeps its call and drops the literal siblings [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, literalTest).Unit
		assertContains(t, unit, `t.Run("case"`, "ErrTooHeavy")
		assertAbsent(t, unit, "ErrNoID", "accepts", "without identifier", "Validate()); err != nil")
	})
	t.Run("a table-driven subtest keeps the whole function [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, tablePriceTest).Unit
		assertContains(t, unit, "a light local parcel", "a heavy national parcel", "range cases")
	})
	t.Run("a top-level test keeps its whole body [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, "TestValidate").Unit
		assertContains(t, unit, "ErrTooHeavy", "ErrNoID", `t.Run("accepts a parcel at the limit"`)
	})
	t.Run("an uncited literal subtest is selected by its reported name [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, uncitedTest).Unit
		assertContains(t, unit, `t.Run("case"`, "ErrNoID")
		assertAbsent(t, unit, "ErrTooHeavy")
	})
	t.Run("a uniquifier suffix does not change the selection [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, uncitedTest+"#01").Unit
		assertContains(t, unit, `t.Run("case"`, "ErrNoID")
	})
	t.Run("two literals with the same reported name keep the whole function [verifies SP-323~1]", func(t *testing.T) {
		source := `package parcel

import "testing"

func TestTwice(t *testing.T) {
	t.Run("same name", func(t *testing.T) { first() })
	t.Run("same_name", func(t *testing.T) { second() })
}

func first()  {}
func second() {}
`
		unit := maskProject(t, moduleProject(source), internalFile, "TestTwice/same_name").Unit
		assertContains(t, unit, "helper1()", "helper2()")
	})
	t.Run("siblings stay when their name is not a literal [verifies SP-323~1]", func(t *testing.T) {
		source := `package parcel

import "testing"

func TestMixed(t *testing.T) {
	t.Run("alpha", func(t *testing.T) { first() })
	for _, name := range []string{"x"} {
		t.Run(name, func(t *testing.T) { second() })
	}
	t.Run("beta", func(t *testing.T) { third() })
}

func first()  {}
func second() {}
func third()  {}
`
		unit := maskProject(t, moduleProject(source), internalFile, "TestMixed/alpha").Unit
		assertContains(t, unit, `t.Run("case"`, "t.Run(name,", "helper1()", "helper2()")
		assertAbsent(t, unit, "beta", "helper3")
	})
}

func TestHelpers(t *testing.T) {
	t.Run("helpers are taken transitively from every test file of the package [verifies SP-323~1]", func(t *testing.T) {
		masked := maskFixture(t, internalFile, tablePriceTest)
		assertContains(t, masked.Unit, "func helper3(t *testing.T, p Parcel) int", "type Helper4 struct")
		assertContains(t, masked.Unit, "func helper2(weight int, zone Zone) Parcel", `const helper1 = "P-1042"`)
		assertAbsent(t, masked.Unit, "neverUsed", "recorder", "defaultRates")
		if want := []string{"helpers_test.go", "parcel_test.go"}; !slices.Equal(masked.TestFiles, want) {
			t.Errorf("TestFiles = %v, want %v", masked.TestFiles, want)
		}
	})
	t.Run("a type brings its methods [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, helperHeavyTest).Unit
		assertContains(t, unit, "func (r *Helper2) add(", "func (r *Helper2) text() string", "type Helper2 struct")
	})
	t.Run("declarations stay in source order [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, helperHeavyTest).Unit
		positions := []int{
			strings.Index(unit, "const helper1"),
			strings.Index(unit, "type Helper2 struct"),
			strings.Index(unit, "func helper3("),
			strings.Index(unit, "func TestCase("),
		}
		for index := 1; index < len(positions); index++ {
			if positions[index-1] < 0 || positions[index-1] >= positions[index] {
				t.Fatalf("declarations out of order: %v\n%s", positions, unit)
			}
		}
	})
	t.Run("an external test package takes helpers from its own files only [verifies SP-323~1]", func(t *testing.T) {
		masked := maskFixture(t, externalFile, externalTest)
		assertContains(t, masked.Unit, "func helper1(weight int) (int, error)")
		assertAbsent(t, masked.Unit, "newParcel", "recorder")
		if want := []string{externalFile}; !slices.Equal(masked.TestFiles, want) {
			t.Errorf("TestFiles = %v, want %v", masked.TestFiles, want)
		}
	})
	t.Run("grouped const specs keep their iota sequence [verifies SP-323~1]", func(t *testing.T) {
		source := `package parcel

import "testing"

const (
	small = iota
	large
)

func TestSizes(t *testing.T) {
	if large != 1 {
		t.Fatal(small)
	}
}
`
		unit := maskProject(t, moduleProject(source), internalFile, "TestSizes").Unit
		assertContains(t, unit, "helper1 = iota", "\thelper2\n")
	})
}

func TestImports(t *testing.T) {
	t.Run("only the imports the unit uses are kept [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, helperHeavyTest).Unit
		assertContains(t, unit, "import (\n\t\"fmt\"\n\t\"strings\"\n\t\"testing\"\n)")
		assertAbsent(t, unit, `"errors"`, "rate")
	})
	t.Run("one block lists the standard library first and sorts each group [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, "TestPriceMatchesTable").Unit
		assertContains(t, unit, "import (\n\t\"testing\"\n\n\t\"example.com/parcel/rate\"\n)")
	})
	t.Run("aliases are kept and blank or dot imports are dropped [verifies SP-323~1]", func(t *testing.T) {
		source := `package parcel

import (
	str "strings"
	_ "embed"
	. "errors"
	"gopkg.in/yaml.v3"
	"testing"
)

func TestImports(t *testing.T) {
	_ = str.ToUpper("a")
	_ = yaml.Marshal
	_ = New
}
`
		unit := maskProject(t, moduleProject(source), internalFile, "TestImports").Unit
		assertContains(t, unit, "import (\n\tstr \"strings\"\n\t\"testing\"\n\n\t\"gopkg.in/yaml.v3\"\n)")
		assertAbsent(t, unit, "embed", `"errors"`)
	})
	t.Run("a unit without imports prints none [verifies SP-323~1]", func(t *testing.T) {
		source := "package parcel\n\nfunc TestPlain() {}\n"
		unit := maskProject(t, moduleProject(source), internalFile, "TestPlain").Unit
		if want := "func TestCase() {}\n"; unit != want {
			t.Errorf("unit = %q, want %q", unit, want)
		}
	})
}

func TestRemovals(t *testing.T) {
	t.Run("comments never reach the unit [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, "TestPriceMatchesTable").Unit
		assertAbsent(t, unit, "//", "source of truth", "/*")
	})
	t.Run("citation markers leave the string literals [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, tablePriceTest).Unit
		assertAbsent(t, unit, "verifies", "PRC-12")
		assertContains(t, unit, `t.Run(c.name+"", func`)
	})
	t.Run("a citation inside a longer literal is cut with its leading space [verifies SP-323~1]", func(t *testing.T) {
		source := strings.ReplaceAll("package parcel\n\nfunc TestNote() {\n\tprintln(\"first part CITE second\", `raw CITE`)\n}\n", "CITE", cite("PRC-9~2"))
		unit := maskProject(t, moduleProject(source), internalFile, "TestNote").Unit
		assertContains(t, unit, `"first part second"`, "`raw`")
	})
	t.Run("a criterion id outside a citation is left to the leak check [verifies SP-323~1]", func(t *testing.T) {
		source := "package parcel\n\nfunc TestBare() {\n\tprintln(\"see PRC-77~1\")\n}\n"
		unit := maskProject(t, moduleProject(source), internalFile, "TestBare").Unit
		assertContains(t, unit, "PRC-77~1")
	})
	t.Run("other literals are unchanged [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, helperHeavyTest).Unit
		assertContains(t, unit, `"label=%s"`, `"label=P-1042/1"`, `"label line = %q"`)
	})
}

func TestNeutralNames(t *testing.T) {
	t.Run("the test and the helpers get neutral names [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, tablePriceTest).Unit
		assertContains(t, unit, "func TestCase(t *testing.T)", "cases := []Helper4{")
		assertAbsent(t, unit, "TestPrice", "newParcel", "mustPrice", "priceCase", "sampleID")
	})
	t.Run("a literal subtest name becomes case [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, literalTest).Unit
		assertContains(t, unit, `t.Run("case", func(t *testing.T) {`)
		assertAbsent(t, unit, "rejects")
	})
	t.Run("a table row that repeats the reported name becomes case [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, tablePriceTest).Unit
		assertContains(t, unit, `{name: "case", weight: 0`)
		assertContains(t, unit, `{name: "a light local parcel"`)
	})
	t.Run("fields, selectors, methods and labels keep their names [verifies SP-323~1]", func(t *testing.T) {
		source := `package parcel

import "testing"

type item struct{ weight int }

var weight = 3

const kind = "a"

func (i item) heavy() bool { return i.weight > weight }

func TestKeys(t *testing.T) {
	values := map[string]int{kind: 1}
	one := item{weight: weight}
outer:
	for range values {
		if one.heavy() {
			break outer
		}
	}
}
`
		unit := maskProject(t, moduleProject(source), internalFile, "TestKeys").Unit
		assertContains(t, unit, "type Helper1 struct{ weight int }", "var helper2 = 3", `const helper3 = "a"`)
		assertContains(t, unit, "func (i Helper1) heavy() bool { return i.weight > helper2 }")
		assertContains(t, unit, "map[string]int{helper3: 1}", "Helper1{weight: helper2}", "one.heavy()", "break outer")
	})
	t.Run("a local variable that shadows a package-level name is not renamed [verifies SP-323~1]", func(t *testing.T) {
		source := `package parcel

import "testing"

var limit = 5

func TestShadow(t *testing.T) {
	limit := 2
	_ = limit
}

func TestOther(t *testing.T) { _ = limit }
`
		unit := maskProject(t, moduleProject(source), internalFile, "TestShadow").Unit
		assertContains(t, unit, "limit := 2")
		assertAbsent(t, unit, "helper1")
	})
}

func TestRenderedUnit(t *testing.T) {
	names := []struct{ file, name string }{
		{internalFile, tablePriceTest},
		{internalFile, literalTest},
		{internalFile, uncitedTest},
		{internalFile, helperHeavyTest},
		{internalFile, "TestPriceMatchesTable"},
		{externalFile, externalTest},
	}
	t.Run("every unit parses behind a package clause and is gofmt-clean [verifies SP-323~1]", func(t *testing.T) {
		for _, c := range names {
			unit := maskFixture(t, c.file, c.name).Unit
			assertParses(t, unit)
			assertFormatted(t, unit)
		}
	})
	t.Run("a unit ends with a single newline and holds no package clause [verifies SP-323~1]", func(t *testing.T) {
		for _, c := range names {
			unit := maskFixture(t, c.file, c.name).Unit
			if unit[len(unit)-1] != '\n' || (len(unit) > 1 && unit[len(unit)-2] == '\n') {
				t.Errorf("%s does not end with exactly one newline: %q", c.name, unit[len(unit)-3:])
			}
			assertAbsent(t, unit, "package ")
		}
	})
	t.Run("declarations are separated by one blank line [verifies SP-323~1]", func(t *testing.T) {
		unit := maskFixture(t, internalFile, helperHeavyTest).Unit
		assertAbsent(t, unit, "\n\n\n")
		assertContains(t, unit, ")\n\nconst helper1", "}\n\nfunc TestCase")
	})
	t.Run("masking is deterministic and mutates nothing shared [verifies SP-323~1]", func(t *testing.T) {
		project := fixture()
		first, err := New().Mask(project, bound(internalFile, tablePriceTest))
		if err != nil {
			t.Fatal(err)
		}
		for range 3 {
			again, err := New().Mask(project, bound(internalFile, tablePriceTest))
			if err != nil || again.Unit != first.Unit || again.Signatures != first.Signatures {
				t.Fatalf("masking differs between runs (err %v)", err)
			}
		}
	})
	t.Run("a project rooted below the module still finds its tests [verifies SP-323~1]", func(t *testing.T) {
		project := fstest.MapFS{}
		for name, file := range moduleProject("package parcel\n\nfunc TestRooted() {}\n") {
			project["sub/dir/"+name] = file
		}
		masked := maskProject(t, project, "sub/dir/"+internalFile, "TestRooted")
		if masked.Unit != "func TestCase() {}\n" {
			t.Errorf("unit = %q", masked.Unit)
		}
	})
}
