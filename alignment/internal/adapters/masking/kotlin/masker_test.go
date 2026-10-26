package kotlin_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/adapters/masking/kotlin"
	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
)

func TestServesKotlinFiles(t *testing.T) {
	t.Run("the masker serves Kotlin files and refuses the others [verifies SP-1224~1]", func(t *testing.T) {
		masker := kotlin.New()
		if !masker.Handles("src/test/kotlin/A.kt") {
			t.Error("a Kotlin file is not served")
		}
		for _, file := range []string{"a_test.py", "a_test.go", "A.java", "a.test.ts"} {
			if masker.Handles(file) {
				t.Errorf("%s is served", file)
			}
		}
		_, err := masker.Mask(sample("x"), criterion.BoundTest{Name: "x", File: "a_test.go"})
		if !errors.Is(err, ports.ErrUnsupportedFile) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestMasksATest(t *testing.T) {
	t.Run("the display name, the comments and the citation tags are gone and the helpers have neutral names [verifies SP-1220~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"package shop",
			"",
			"import org.junit.jupiter.api.DisplayName",
			"import org.junit.jupiter.api.Test",
			"import org.junit.jupiter.api.Assertions.assertEquals",
			"",
			"const val UNIT = 4 // per unit",
			"",
			"fun doubled(value: Int): Int = value * 2",
			"",
			"val UNUSED = 9",
			"",
			"/** Prices a line. */",
			"class SampleTest {",
			"    @Test",
			tagged("    @DisplayName(\"doubles a unit [cites PAR-9~1]\")"),
			"    fun doublesAUnit() {",
			"        // two for one",
			"        assertEquals(8, doubled(UNIT))",
			"    }",
			"",
			"    @Test",
			"    fun somethingElse() {",
			"        assertEquals(UNUSED, 9)",
			"    }",
			"}",
		), sampleTest(sampleClass, tagged("doubles a unit [cites PAR-9~1]")))
		assertEqual(t, got, lines(
			"import org.junit.jupiter.api.Test",
			"import org.junit.jupiter.api.Assertions.assertEquals",
			"",
			"const val helper1 = 4",
			"",
			"fun helper2(value: Int): Int = value * 2",
			"",
			"class Case {",
			"    @Test",
			"    fun testCase() {",
			"        assertEquals(8, helper2(helper1))",
			"    }",
			"}",
		))
	})

	t.Run("a class that several annotations precede is read like any other, and its annotations that stay are shown [verifies SP-1220~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"@ExtendWith(MockitoExtension::class)",
			"@DisplayName(\"the cart\")",
			"@Tag(\"fast\")",
			"class SampleTest {",
			"    val cart = Cart()",
			"",
			"    @Nested @DisplayName(\"with items\") inner class Items {",
			"        @Test fun counts() { assertEquals(0, cart.size) }",
			"    }",
			"}",
		), sampleTest(sampleClass+"$Items", "counts()"))
		assertEqual(t, got, lines(
			"@ExtendWith(MockitoExtension::class)",
			"class Case {",
			"    val helper1 = Cart()",
			"",
			"    @Test fun testCase() { assertEquals(0, helper1.size) }",
			"}",
		))
	})

	t.Run("the hooks and the extensions of the classes around the test stay [verifies SP-1220~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"class SampleTest {",
			"    @RegisterExtension",
			"    val clock = Clock()",
			"    lateinit var cart: Cart",
			"    lateinit var other: Cart",
			"",
			"    init { setup() }",
			"",
			"    @BeforeEach",
			"    fun open() { cart = Cart() }",
			"",
			"    @Test",
			"    fun counts() { assertEquals(0, cart.size) }",
			"",
			"    @Test",
			"    fun empty() { assertEquals(0, other.size) }",
			"}",
		), sampleTest(sampleClass, "counts()"))
		assertEqual(t, got, lines(
			"class Case {",
			"    @RegisterExtension",
			"    val helper1 = Clock()",
			"",
			"    lateinit var helper2: Cart",
			"",
			"    init { setup() }",
			"",
			"    @BeforeEach",
			"    fun helper3() { helper2 = Cart() }",
			"",
			"    @Test",
			"    fun testCase() { assertEquals(0, helper2.size) }",
			"}",
		))
	})

	t.Run("a call on another object keeps its name even when a helper is called the same [verifies SP-1220~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"class SampleTest {",
			"    val value = 1",
			"    fun clear() {}",
			"",
			"    @Test",
			"    fun works() {",
			"        clear()",
			"        list.clear()",
			"        this.clear()",
			"        other.value = this.value",
			"        println(\"$value ${other.value}\")",
			"    }",
			"}",
		), sampleTest(sampleClass, "works()"))
		for _, want := range []string{"helper2()", "list.clear()", "this.helper2()", "other.value = this.helper1", "${helper1} ${other.value}"} {
			if !strings.Contains(got, want) {
				t.Errorf("no %q in:\n%s", want, got)
			}
		}
	})

	t.Run("the method a source annotation names is reached in a companion object and renamed with its reference [verifies SP-1220~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"class SampleTest {",
			"    companion object {",
			"        @JvmStatic",
			"        fun rows(): Stream<Arguments> = Stream.of()",
			"    }",
			"",
			"    @ParameterizedTest",
			"    @MethodSource(\"rows\")",
			"    fun reads(row: Int) {}",
			"}",
		), sampleTest(sampleClass, "reads(int)[1]"))
		if !strings.Contains(got, `@MethodSource("helper1")`) || !strings.Contains(got, "fun helper1(): Stream<Arguments>") {
			t.Fatalf("got:\n%s", got)
		}
	})

	t.Run("only the name pattern of a parametrized test goes, its other arguments stay [verifies SP-1220~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"class SampleTest {",
			tagged("    @ParameterizedTest(name = \"{0} [cites PAR-4~1]\", quoteCharacter = 'x')"),
			"    @ValueSource(ints = [1])",
			"    fun many(n: Int) {}",
			"",
			tagged("    @RepeatedTest(value = 3, name = \"run {currentRepetition} [cites PAR-4~1]\")"),
			"    fun again() {}",
			"}",
		), sampleTest(sampleClass, tagged("7 [cites PAR-4~1]")))
		if !strings.Contains(got, "@ParameterizedTest(quoteCharacter = 'x')") || strings.Contains(got, "PAR-4") {
			t.Fatalf("got:\n%s", got)
		}
	})
}

func TestFindsTheBoundTest(t *testing.T) {
	const source = `package shop

class SampleTest {
    @Test
    @DisplayName("adds up [cites PAR-1~1]")
    fun addsUp() { first() }

    @Test
    fun ` + "`the plain one`" + `() { second() }

    @ParameterizedTest
    fun defaults(n: Int) { third() }

    @ParameterizedTest(name = "{0} costs {1} [cites PAR-2~1]")
    fun named(n: Int, m: Int) { fourth() }

    @Nested
    @DisplayName("bulk orders [cites PAR-3~1]")
    inner class Bulk {
        @Test
        fun addsUp() { fifth() }
    }
}
`
	find := func(t *testing.T, classname, name string) string {
		t.Helper()
		return mustMask(t, sample(source), criterion.BoundTest{Classname: classname, Name: name, File: sampleTestFile}).Unit
	}

	t.Run("a test is found by its display name with the citation, by its function name with or without its signature, backticks or not [verifies SP-1222~1]", func(t *testing.T) {
		for _, name := range []string{tagged("adds up [cites PAR-1~1]"), "adds up", "addsUp", "addsUp()"} {
			if got := find(t, sampleClass, name); !strings.Contains(got, "first()") {
				t.Errorf("%s: %s", name, got)
			}
		}
		for _, name := range []string{"the plain one", "the plain one()"} {
			if got := find(t, sampleClass, name); !strings.Contains(got, "second()") {
				t.Errorf("%s: %s", name, got)
			}
		}
	})

	t.Run("a parametrized test is found by its name pattern expanded and by the default name of an invocation [verifies SP-1222~1]", func(t *testing.T) {
		if got := find(t, sampleClass, tagged("7 costs 9 [cites PAR-2~1]")); !strings.Contains(got, "fourth()") {
			t.Errorf("pattern: %s", got)
		}
		if got := find(t, sampleClass, "[1] 10"); !strings.Contains(got, "third()") {
			t.Errorf("default: %s", got)
		}
	})

	t.Run("the class tells two tests of one name apart, as a binary name or as the display names a reporter writes [verifies SP-1222~1]", func(t *testing.T) {
		if got := find(t, sampleClass+"$Bulk", "addsUp()"); !strings.Contains(got, "fifth()") {
			t.Errorf("binary name: %s", got)
		}
		if got := find(t, tagged("SampleTest bulk orders [cites PAR-3~1]"), "addsUp"); !strings.Contains(got, "fifth()") {
			t.Errorf("display names: %s", got)
		}
		if got := find(t, sampleClass, "addsUp()"); !strings.Contains(got, "first()") {
			t.Errorf("outer: %s", got)
		}
	})

	t.Run("a test the file does not declare is not found [verifies SP-1222~1]", func(t *testing.T) {
		_, err := kotlin.New().Mask(sample(source), sampleTest(sampleClass, "nothing like it"))
		if !errors.Is(err, ports.ErrTestNotFound) {
			t.Fatalf("error = %v", err)
		}
		_, err = kotlin.New().Mask(sample(source), criterion.BoundTest{Name: "x", File: "src/test/kotlin/Missing.kt"})
		if err == nil || errors.Is(err, ports.ErrTestNotFound) {
			t.Fatalf("a missing file gave %v", err)
		}
	})
}

func TestRefusesATestThatSpellsItsName(t *testing.T) {
	t.Run("a string that spells the display name makes the masked input leak [verifies SP-1223~1]", func(t *testing.T) {
		_, err := kotlin.New().Mask(sample(lines(
			"class SampleTest {",
			"    @Test",
			"    @DisplayName(\"grace period is free\")",
			"    fun freePeriod() {",
			"        val label = \"grace period is free\"",
			"    }",
			"}",
		)), sampleTest(sampleClass, "grace period is free"))
		if !errors.Is(err, masking.ErrLeak) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a sentence of a backticked name that the code spells makes it leak [verifies SP-1223~1]", func(t *testing.T) {
		_, err := kotlin.New().Mask(sample(lines(
			"class SampleTest {",
			"    @Test",
			"    fun `grace period is free`() {",
			"        audit.gracePeriodIsFree()",
			"    }",
			"}",
		)), sampleTest(sampleClass, "grace period is free()"))
		if !errors.Is(err, masking.ErrLeak) {
			t.Fatalf("error = %v", err)
		}
	})
}
