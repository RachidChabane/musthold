package jvm_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/jvm"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/musthold/discpower/internal/app/execution"
	"github.com/RachidChabane/musthold/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/discpower/internal/testkit/jvmenv"
)

const (
	mavenRatesFile  = "src/main/java/com/example/parcel/Rates.java"
	gradleRatesFile = "src/main/java/parcel/Rates.java"
	kotlinRatesFile = "src/main/kotlin/parcel/Rates.kt"

	jvmShippingSpec = "WHEN an order total reaches 50 THE SYSTEM SHALL waive the shippingFee."

	freeWitnessName = "check_free_from_fifty"
	feeWitnessName  = "check_fee_below_fifty"

	mavenImports  = "import static org.junit.jupiter.api.Assertions.assertEquals;\nimport java.math.BigDecimal;"
	mavenFreeCode = "void check_free_from_fifty() {\n    assertEquals(0, BigDecimal.ZERO.compareTo(Rates.shippingFee(new BigDecimal(\"50.00\"))));\n}\n"
	mavenFeeCode  = "@Test\nvoid check_fee_below_fifty() {\n    assertEquals(0, Rates.FLAT_FEE.compareTo(Rates.shippingFee(new BigDecimal(\"49.99\"))));\n}\n"

	gradleImports  = "static org.junit.jupiter.api.Assertions.assertEquals"
	gradleFreeCode = "void check_free_from_fifty() {\n    assertEquals(0, Rates.shippingFee(Rates.FREE_SHIPPING_FROM));\n}\n"
	gradleFeeCode  = "void check_fee_below_fifty() {\n    assertEquals(Rates.FLAT_FEE, Rates.shippingFee(Rates.FREE_SHIPPING_FROM - 1));\n}\n"

	kotlinImports  = "import org.junit.jupiter.api.Assertions.assertEquals"
	kotlinFreeCode = "fun check_free_from_fifty() {\n    assertEquals(0, shippingFee(FREE_SHIPPING_FROM))\n}\n"
	kotlinFeeCode  = "fun check_fee_below_fifty() {\n    assertEquals(FLAT_FEE, shippingFee(FREE_SHIPPING_FROM - 1))\n}\n"

	mavenBoundaryEdit  = "public static BigDecimal shippingFee(BigDecimal total) {\n        if (total.compareTo(FREE_SHIPPING_FROM) > 0) {\n            return BigDecimal.ZERO;\n        }\n        return FLAT_FEE;\n    }"
	gradleBoundaryEdit = "public static int shippingFee(int total) {\n        if (total > FREE_SHIPPING_FROM) {\n            return 0;\n        }\n        return FLAT_FEE;\n    }"
	kotlinBoundaryEdit = "fun shippingFee(total: Int): Int {\n    if (total > FREE_SHIPPING_FROM) {\n        return 0\n    }\n    return FLAT_FEE\n}"
)

var shippingCriterion = spec.Ref{ID: "PAR-3", Revision: 1}

func witnessSet(imports, freeCode, feeCode string) witness.Set {
	return witness.Set{Criterion: shippingCriterion, Imports: imports, Witnesses: []witness.Witness{
		witness.New(shippingCriterion, freeWitnessName, "waive the shipping fee", witness.KindExample, freeCode),
		witness.New(shippingCriterion, feeWitnessName, "reaches 50", witness.KindExample, feeCode),
	}}
}

func fixtureSource(t *testing.T, fixture, file string) mutation.SourceFile {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(fixture, filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	return mutation.SourceFile{Path: file, Content: content}
}

func TestWitnessBrief(t *testing.T) {
	t.Run("a Java brief lists the headers of the public definitions the spec mentions, never a body [verifies SP-1986~1]", func(t *testing.T) {
		source := fixtureSource(t, mavenFixture, mavenRatesFile)
		brief, err := maven(nil, nil).Witnesses().Brief(os.DirFS(mavenFixture), []mutation.SourceFile{source}, jvmShippingSpec)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"com.example.parcel.Rates: public static BigDecimal shippingFee(BigDecimal total)"}; !slices.Equal(brief.Interface, want) {
			t.Errorf("interface %q", brief.Interface)
		}
		if brief.Language != "Java, JUnit 5 on Maven" || !strings.Contains(brief.Conventions, "`com.example.parcel`") {
			t.Errorf("language %q, conventions %q", brief.Language, brief.Conventions)
		}
	})

	t.Run("a spec that mentions no definition gets every public one, types and constants included [verifies SP-1986~1]", func(t *testing.T) {
		source := fixtureSource(t, mavenFixture, mavenRatesFile)
		brief, err := maven(nil, nil).Witnesses().Brief(os.DirFS(mavenFixture), []mutation.SourceFile{source}, "nothing named here")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			"com.example.parcel: public final class Rates",
			"com.example.parcel: public static final BigDecimal FREE_SHIPPING_FROM",
			"com.example.parcel: public static final BigDecimal FLAT_FEE",
			"com.example.parcel: public static final int MAX_PERCENT",
			"com.example.parcel.Rates: public static BigDecimal lineTotal(BigDecimal unitPrice, int quantity)",
			"com.example.parcel.Rates: public static BigDecimal discounted(BigDecimal total, int percent)",
			"com.example.parcel.Rates: public static BigDecimal shippingFee(BigDecimal total)",
		}
		if !slices.Equal(brief.Interface, want) {
			t.Errorf("interface %q", brief.Interface)
		}
		for _, entry := range brief.Interface {
			if strings.Contains(entry, "return") || strings.Contains(entry, "new BigDecimal") {
				t.Errorf("a body or a value leaked: %q", entry)
			}
		}
	})

	t.Run("a Kotlin brief lists top-level functions and constants of the package, never an internal one [verifies SP-1986~1]", func(t *testing.T) {
		source := fixtureSource(t, kotlinFixture, kotlinRatesFile)
		project := os.DirFS(kotlinFixture)
		sources := []mutation.SourceFile{source}
		mentioned, err := gradle(nil, nil).Witnesses().Brief(project, sources, jvmShippingSpec)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"parcel: fun shippingFee(total: Int): Int"}; !slices.Equal(mentioned.Interface, want) {
			t.Errorf("interface %q", mentioned.Interface)
		}
		if mentioned.Language != "Kotlin, JUnit 5 on Gradle" {
			t.Errorf("language %q", mentioned.Language)
		}
		every, err := gradle(nil, nil).Witnesses().Brief(project, sources, "nothing named here")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{
			"parcel: const val FREE_SHIPPING_FROM", "parcel: const val FLAT_FEE", "parcel: const val MAX_PERCENT",
			"parcel: fun lineTotal(unitPrice: Int, quantity: Int): Int", "parcel: fun discounted(total: Int, percent: Int): Int",
			"parcel: fun shippingFee(total: Int): Int",
		}
		if !slices.Equal(every.Interface, want) {
			t.Errorf("interface %q", every.Interface)
		}
	})

	t.Run("the helpers of every test file are listed by signature, tests and private methods left out [verifies SP-1986~1]", func(t *testing.T) {
		project := fstest.MapFS{
			"src/test/java/p/RatesTest.java": {Data: []byte("package p;\n\nclass RatesTest {\n    @Test\n    void works() {\n        assertTrue(true);\n    }\n\n    static int fixtureTotal(int base) {\n        return base * 2;\n    }\n\n    private int secret() {\n        return 1;\n    }\n}\n")},
			"src/test/java/p/Support.java":   {Data: []byte("package p;\n\npublic class Support {\n    @BeforeEach\n    void setUp() {\n    }\n\n    public static String label() {\n        return \"x\";\n    }\n}\n")},
			"src/test/kotlin/q/KitTest.kt":   {Data: []byte("package q\n\nclass KitTest {\n    @Test\n    fun works() {\n    }\n\n    fun helper(x: Int): Int = x + 1\n}\n")},
		}
		brief, err := maven(nil, nil).Witnesses().Brief(project, []mutation.SourceFile{{Path: mavenRatesFile, Content: fixtureSource(t, mavenFixture, mavenRatesFile).Content}}, "nothing")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"p.Support: public static String label()", "p.RatesTest: static int fixtureTotal(int base)", "q.KitTest: fun helper(x: Int): Int"}
		slices.Sort(want)
		got := slices.Clone(brief.Helpers)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("helpers %q", brief.Helpers)
		}
	})
}

var renamedMethod = regexp.MustCompile(`musthold_witness_[0-9a-f]{12}`)

func TestWitnessLayout(t *testing.T) {
	cases := []struct {
		name      string
		strategy  *jvm.Strategy
		fixture   string
		directory string
		pkg       string
		extension string
		imports   string
		freeCode  string
		feeCode   string
		selection string
	}{
		{"java on maven", maven(nil, nil), mavenFixture, "src/test/java/com/example/parcel/", "com.example.parcel", ".java", mavenImports, mavenFreeCode, mavenFeeCode, "-Dtest="},
		{"java on gradle", gradle(nil, nil), gradleFixture, "src/test/java/parcel/", "parcel", ".java", gradleImports, gradleFreeCode, gradleFeeCode, "--tests"},
		{"kotlin on gradle", gradle(nil, nil), kotlinFixture, "src/test/kotlin/parcel/", "parcel", ".kt", kotlinImports, kotlinFreeCode, kotlinFeeCode, "--tests"},
	}
	for _, c := range cases {
		project := os.DirFS(c.fixture)
		set := witnessSet(c.imports, c.freeCode, c.feeCode)

		t.Run(c.name+": a set is laid out as a test class of the package of the code under test, each witness renamed, annotated and named as JUnit names it [verifies SP-1986~1]", func(t *testing.T) {
			file, err := c.strategy.Witnesses().Layout(project, set)
			if err != nil {
				t.Fatal(err)
			}
			className := strings.TrimSuffix(filepath.Base(file.Path), c.extension)
			if !strings.HasPrefix(file.Path, c.directory+"MustholdWitness") || !strings.HasSuffix(className, "Test") || !strings.HasSuffix(file.Path, c.extension) {
				t.Fatalf("path %q", file.Path)
			}
			content := string(file.Content)
			if !strings.HasPrefix(content, "package "+c.pkg) || strings.Contains(content, " "+freeWitnessName+"(") {
				t.Errorf("content %q", content)
			}
			if strings.Count(content, "@Test\n") != 2 || !strings.Contains(content, "import org.junit.jupiter.api.Test") {
				t.Errorf("annotations missing or duplicated: %q", content)
			}
			lines := strings.Split(content, "\n")
			for _, w := range set.Witnesses {
				test, known := file.Tests[w.ID]
				if !known || test.Classname != c.pkg+"."+className || test.File != file.Path || !strings.HasSuffix(test.Name, w.ShortID()) {
					t.Fatalf("test %+v of %s", test, w.Name)
				}
				if !renamedMethod.MatchString(test.Name) || !strings.Contains(lines[test.Line-1], test.Name+"(") {
					t.Errorf("line %d is %q", test.Line, lines[test.Line-1])
				}
				if !strings.Contains(content, "@DisplayName(\""+test.Name+"\")") {
					t.Errorf("no display name for %s", test.Name)
				}
			}
		})

		t.Run(c.name+": the selector maps every laid-out witness to its method [verifies SP-1986~1]", func(t *testing.T) {
			file, err := c.strategy.Witnesses().Layout(project, set)
			if err != nil {
				t.Fatal(err)
			}
			definitions, err := treesitter.NewLocator().Locate(file.Path, file.Content)
			if err != nil {
				t.Fatal(err)
			}
			var bound []ecosystem.BoundTest
			for _, w := range set.Witnesses {
				bound = append(bound, file.Tests[w.ID])
			}
			selection, err := c.strategy.Selector().Select(bound, map[string][]ecosystem.Definition{file.Path: definitions})
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(selection.Args, " ")
			for _, test := range bound {
				if !strings.Contains(joined, test.Name) || !strings.Contains(joined, test.Classname) {
					t.Errorf("%s is not selected by %q", test.Name, joined)
				}
			}
			if !strings.Contains(joined, c.selection) {
				t.Errorf("selection %q", joined)
			}
		})

		t.Run(c.name+": a witness that does not declare its function is left out [verifies SP-1986~1]", func(t *testing.T) {
			broken := witnessSet(c.imports, "assertEquals(1, 1);\n", c.feeCode)
			file, err := c.strategy.Witnesses().Layout(project, broken)
			if err != nil {
				t.Fatal(err)
			}
			if _, known := file.Tests[broken.Witnesses[0].ID]; known || len(file.Tests) != 1 {
				t.Errorf("tests %+v", file.Tests)
			}
		})

		t.Run(c.name+": a laid-out class is a test file of the ecosystem [verifies SP-1986~1]", func(t *testing.T) {
			file, err := c.strategy.Witnesses().Layout(project, set)
			if err != nil {
				t.Fatal(err)
			}
			if !c.strategy.Layout().IsTestFile(file.Path) {
				t.Errorf("%s is not a test file", file.Path)
			}
		})
	}

	t.Run("a brief places the witnesses in the package the spec mentions most [verifies SP-1986~1]", func(t *testing.T) {
		project := fstest.MapFS{}
		sources := []mutation.SourceFile{
			{Path: "src/main/java/a/Alpha.java", Content: []byte("package a;\n\npublic class Alpha {\n    public int one() {\n        return 1;\n    }\n}\n")},
			{Path: "src/main/java/b/Beta.java", Content: []byte("package b;\n\npublic class Beta {\n    public int two() {\n        return 2;\n    }\n}\n")},
		}
		strategy := maven(nil, nil)
		if _, err := strategy.Witnesses().Brief(project, sources, "the two of Beta"); err != nil {
			t.Fatal(err)
		}
		file, err := strategy.Witnesses().Layout(project, witnessSet("", "void check_free_from_fifty() {\n}\n", "void check_fee_below_fifty() {\n}\n"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(file.Path, "src/test/java/b/MustholdWitness") {
			t.Errorf("path %q", file.Path)
		}
	})

	t.Run("a module of a multi-module build keeps its witnesses in its own test source set [verifies SP-1986~1]", func(t *testing.T) {
		project := fstest.MapFS{"core/src/main/java/a/Alpha.java": {Data: []byte("package a;\n\npublic class Alpha {\n}\n")}}
		file, err := maven(nil, nil).Witnesses().Layout(project, witnessSet("", "void check_free_from_fifty() {\n}\n", "void check_fee_below_fifty() {\n}\n"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(file.Path, "core/src/test/java/a/MustholdWitness") {
			t.Errorf("path %q", file.Path)
		}
	})
}

// jvmLiveCase is one build tool and language of the fixtures.
type jvmLiveCase struct {
	name     string
	fixture  string
	source   string
	qualname string
	edit     string
	imports  string
	free     string
	fee      string
	fetch    func(*testing.T, string)
	build    func(jvm.Dependencies) *jvm.Strategy
}

func liveCases() []jvmLiveCase {
	return []jvmLiveCase{
		{"java on maven", mavenFixture, mavenRatesFile, "Rates.shippingFee", mavenBoundaryEdit, mavenImports, mavenFreeCode, mavenFeeCode, jvmenv.FetchMaven, jvm.NewMaven},
		{"java on gradle", gradleFixture, gradleRatesFile, "Rates.shippingFee", gradleBoundaryEdit, gradleImports, gradleFreeCode, gradleFeeCode, jvmenv.FetchGradle, jvm.NewGradle},
		{"kotlin on gradle", kotlinFixture, kotlinRatesFile, "shippingFee", kotlinBoundaryEdit, kotlinImports, kotlinFreeCode, kotlinFeeCode, jvmenv.FetchGradle, jvm.NewGradle},
	}
}

// liveRunner prepares the fixture as a reference and returns a runner of its
// witnesses.
func (c jvmLiveCase) liveRunner(t *testing.T) execution.Runner {
	t.Helper()
	c.fetch(t, c.fixture)
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	strategy := c.build(jvm.Dependencies{Process: process.New(), Reader: junitxml.NewReader(), Locator: treesitter.NewLocator()})
	executor := mutantrun.Executor{Ecosystem: strategy, Workspaces: manager, Policy: mutation.Policy{SourceRoots: []string{"src/main"}, Layout: strategy.Layout()}}
	reference, err := executor.PrepareReference(context.Background(), filepath.Clean(c.fixture))
	if err != nil {
		t.Fatalf("prepare the reference: %v", err)
	}
	built, err := executor.BuildReference(context.Background(), reference, []string{c.source})
	if err != nil || !built.OK {
		t.Fatalf("the reference does not build: %v %s", err, built.Diagnostics)
	}
	return execution.Runner{
		Executor: executor, Reference: reference,
		Witnesses: execution.Witnesses{Host: strategy.Witnesses(), Project: os.DirFS(filepath.Clean(c.fixture))},
	}
}

func TestWitnessRuns(t *testing.T) {
	for _, c := range liveCases() {
		t.Run(c.name, func(t *testing.T) {
			jvmenv.Require(t)
			runner := c.liveRunner(t)
			ctx := context.Background()
			set := witnessSet(c.imports, c.free, c.fee)

			t.Run("a laid-out witness passes the reference three times out of three [verifies SP-1986~1]", func(t *testing.T) {
				runs, err := runner.ReferenceWitnesses(ctx, set)
				if err != nil {
					t.Fatal(err)
				}
				for _, w := range set.Witnesses {
					if witness.ReadingOf(runs[w.ID]) != witness.ReadingPasses {
						t.Errorf("%s ran %v", w.Name, runs[w.ID])
					}
				}
			})

			t.Run("a mutant that breaks what the witness checks fails it, reproduced [verifies SP-1986~1]", func(t *testing.T) {
				edits := mutation.EditSet{Edits: []mutation.Edit{{File: c.source, Qualname: c.qualname, Target: mutation.TargetDefinition, NewSource: c.edit}}}
				outcomes, err := runner.EditWitnesses(ctx, "boundary", edits, []witness.Set{set})
				if err != nil {
					t.Fatal(err)
				}
				if outcomes[set.Witnesses[0].ID] != witness.OutcomeFailed || outcomes[set.Witnesses[1].ID] != witness.OutcomePassed {
					t.Errorf("outcomes %v", outcomes)
				}
			})
		})
	}
}
