package java_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/masking/java"
	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/masking"
)

func TestServesJavaFiles(t *testing.T) {
	t.Run("the masker serves Java files and refuses the others [verifies SP-1214~1]", func(t *testing.T) {
		masker := java.New()
		if !masker.Handles("src/test/java/A.java") {
			t.Error("a Java file is not served")
		}
		for _, file := range []string{"a_test.py", "a_test.go", "A.kt", "a.test.ts"} {
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
	t.Run("the display name, the comments and the citation tags are gone and the helpers have neutral names [verifies SP-1210~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"package shop;",
			"",
			"import org.junit.jupiter.api.DisplayName;",
			"import org.junit.jupiter.api.Test;",
			"import static org.junit.jupiter.api.Assertions.assertEquals;",
			"",
			"/** Prices a line. */",
			"class SampleTest {",
			"    static final int UNIT = 4; // per unit",
			"",
			"    static int doubled(int value) {",
			"        return value * 2;",
			"    }",
			"",
			"    static final int UNUSED = 9;",
			"",
			"    @Test",
			tagged("    @DisplayName(\"doubles a unit [cites PAR-9~1]\")"),
			"    void doublesAUnit() {",
			"        // two for one",
			"        assertEquals(8, doubled(UNIT));",
			"    }",
			"",
			"    @Test",
			"    void somethingElse() {",
			"        assertEquals(UNUSED, 9);",
			"    }",
			"}",
		), sampleTest(sampleClass, tagged("doubles a unit [cites PAR-9~1]")))
		assertEqual(t, got, lines(
			"import org.junit.jupiter.api.Test;",
			"import static org.junit.jupiter.api.Assertions.assertEquals;",
			"",
			"class Case {",
			"    static final int helper1 = 4;",
			"",
			"    static int helper2(int value) {",
			"        return value * 2;",
			"    }",
			"",
			"    @Test",
			"    void testCase() {",
			"        assertEquals(8, helper2(helper1));",
			"    }",
			"}",
		))
	})

	t.Run("the hooks, the extensions and the annotations of the classes around the test stay [verifies SP-1210~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"package shop;",
			"",
			"@ExtendWith(MockitoExtension.class)",
			"@DisplayName(\"the cart\")",
			"class SampleTest {",
			"    @RegisterExtension",
			"    static Clock clock = new Clock();",
			"    Cart cart;",
			"    Cart other;",
			"",
			"    @BeforeEach",
			"    void open() {",
			"        cart = new Cart();",
			"    }",
			"",
			"    @Nested",
			"    @DisplayName(\"with items\")",
			"    class Items {",
			"        @AfterEach",
			"        void close() {",
			"            cart.clear();",
			"        }",
			"",
			"        @Test",
			"        void counts() {",
			"            assertEquals(0, cart.size());",
			"        }",
			"    }",
			"",
			"    @Test",
			"    void empty() {",
			"        assertEquals(0, other.size());",
			"    }",
			"}",
		), sampleTest(sampleClass+"$Items", "counts()"))
		assertEqual(t, got, lines(
			"@ExtendWith(MockitoExtension.class)",
			"class Case {",
			"    @RegisterExtension",
			"    static Clock helper1 = new Clock();",
			"",
			"    Cart helper2;",
			"",
			"    @BeforeEach",
			"    void helper3() {",
			"        helper2 = new Cart();",
			"    }",
			"",
			"    @AfterEach",
			"    void helper4() {",
			"        helper2.clear();",
			"    }",
			"",
			"    @Test",
			"    void testCase() {",
			"        assertEquals(0, helper2.size());",
			"    }",
			"}",
		))
	})

	t.Run("a call on another object keeps its name even when a helper is called the same [verifies SP-1210~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"class SampleTest {",
			"    int value = 1;",
			"",
			"    void clear() {}",
			"",
			"    @Test",
			"    void works() {",
			"        clear();",
			"        list.clear();",
			"        this.clear();",
			"        other.value = this.value;",
			"    }",
			"}",
		), sampleTest(sampleClass, "works()"))
		for _, want := range []string{"helper2();", "list.clear();", "this.helper2();", "other.value = this.helper1;"} {
			if !strings.Contains(got, want) {
				t.Errorf("no %q in:\n%s", want, got)
			}
		}
	})

	t.Run("the method a source annotation names is reached and renamed with its reference [verifies SP-1210~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"class SampleTest {",
			"    static Stream<Arguments> rows() {",
			"        return Stream.of(Arguments.of(1));",
			"    }",
			"",
			"    @ParameterizedTest",
			"    @MethodSource(\"rows\")",
			"    void reads(int row) {}",
			"}",
		), sampleTest(sampleClass, "reads(int)[1]"))
		if !strings.Contains(got, `@MethodSource("helper1")`) || !strings.Contains(got, "static Stream<Arguments> helper1()") {
			t.Fatalf("got:\n%s", got)
		}
	})

	t.Run("only the name pattern of a parametrized or repeated test goes, its other elements stay [verifies SP-1210~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"class SampleTest {",
			tagged("    @RepeatedTest(value = 3, name = \"run {currentRepetition} [cites PAR-4~1]\")"),
			"    void again() {}",
			"",
			tagged("    @ParameterizedTest(name = \"{0} [cites PAR-4~1]\", quoteCharacter = 'x')"),
			"    @ValueSource(ints = {1})",
			"    void many(int n) {}",
			"}",
		), sampleTest(sampleClass, tagged("run 2 [cites PAR-4~1]")))
		if !strings.Contains(got, "@RepeatedTest(value = 3)") {
			t.Fatalf("got:\n%s", got)
		}
		got = maskUnit(t, lines(
			"class SampleTest {",
			tagged("    @ParameterizedTest(name = \"{0} [cites PAR-4~1]\", quoteCharacter = 'x')"),
			"    @ValueSource(ints = {1})",
			"    void many(int n) {}",
			"}",
		), sampleTest(sampleClass, tagged("7 [cites PAR-4~1]")))
		if !strings.Contains(got, "@ParameterizedTest(quoteCharacter = 'x')") || strings.Contains(got, "PAR-4") {
			t.Fatalf("got:\n%s", got)
		}
	})

	t.Run("a type of the file the test reaches is listed before the class, under a neutral name [verifies SP-1210~1]", func(t *testing.T) {
		got := maskUnit(t, lines(
			"class SampleTest {",
			"    @Test",
			"    void builds() {",
			"        assertEquals(2, new Fixture().size());",
			"    }",
			"}",
			"",
			"class Fixture {",
			"    int size() { return 2; }",
			"}",
			"",
			"class Stray {}",
		), sampleTest(sampleClass, "builds()"))
		assertEqual(t, got, lines(
			"class Helper1 {",
			"    int size() { return 2; }",
			"}",
			"",
			"class Case {",
			"    @Test",
			"    void testCase() {",
			"        assertEquals(2, new Helper1().size());",
			"    }",
			"}",
		))
	})
}

func TestFindsTheBoundTest(t *testing.T) {
	const source = `package shop;

class SampleTest {
    @Test
    @DisplayName("adds up [cites PAR-1~1]")
    void addsUp() { first(); }

    @Test
    void plain() { second(); }

    @ParameterizedTest
    void defaults(int n) { third(); }

    @ParameterizedTest(name = "{0} costs {1} [cites PAR-2~1]")
    void named(int n, int m) { fourth(); }

    @RepeatedTest(2)
    void repeated() { fifth(); }

    @Nested
    @DisplayName("bulk orders [cites PAR-3~1]")
    class Bulk {
        @Test
        void plain() { sixth(); }
    }
}
`
	find := func(t *testing.T, classname, name string, line int) string {
		t.Helper()
		test := criterion.BoundTest{Classname: classname, Name: name, File: sampleTestFile, Line: line}
		return mustMask(t, sample(source), test).Unit
	}

	t.Run("a test is found by its display name with the citation, by its method name with or without its signature [verifies SP-1212~1]", func(t *testing.T) {
		for _, name := range []string{tagged("adds up [cites PAR-1~1]"), "adds up", "addsUp", "addsUp()"} {
			if got := find(t, sampleClass, name, 0); !strings.Contains(got, "first()") {
				t.Errorf("%s: %s", name, got)
			}
		}
	})

	t.Run("a parametrized test is found by its name pattern expanded and by the default name of an invocation [verifies SP-1212~1]", func(t *testing.T) {
		if got := find(t, sampleClass, tagged("7 costs 9 [cites PAR-2~1]"), 0); !strings.Contains(got, "fourth()") {
			t.Errorf("pattern: %s", got)
		}
		for _, name := range []string{"[1] 10", "defaults(int)[1]", "defaults(int)"} {
			if got := find(t, sampleClass, name, 0); !strings.Contains(got, "third()") {
				t.Errorf("%s: %s", name, got)
			}
		}
	})

	t.Run("a repeated test is found by the default name of a repetition [verifies SP-1212~1]", func(t *testing.T) {
		if got := find(t, sampleClass, "repetition 2 of 2", 0); !strings.Contains(got, "fifth()") {
			t.Fatalf("got: %s", got)
		}
	})

	t.Run("the class tells two tests of one name apart, as a binary name or as the display names Surefire writes [verifies SP-1212~1]", func(t *testing.T) {
		if got := find(t, sampleClass+"$Bulk", "plain()", 0); !strings.Contains(got, "sixth()") {
			t.Errorf("binary name: %s", got)
		}
		if got := find(t, tagged("SampleTest bulk orders [cites PAR-3~1]"), "plain", 0); !strings.Contains(got, "sixth()") {
			t.Errorf("display names: %s", got)
		}
		if got := find(t, sampleClass, "plain()", 0); !strings.Contains(got, "second()") {
			t.Errorf("outer: %s", got)
		}
	})

	t.Run("a test the file does not declare is not found [verifies SP-1212~1]", func(t *testing.T) {
		_, err := java.New().Mask(sample(source), sampleTest(sampleClass, "nothing like it"))
		if !errors.Is(err, ports.ErrTestNotFound) {
			t.Fatalf("error = %v", err)
		}
		_, err = java.New().Mask(sample(source), criterion.BoundTest{Name: "x", File: "src/test/java/Missing.java"})
		if err == nil || errors.Is(err, ports.ErrTestNotFound) {
			t.Fatalf("a missing file gave %v", err)
		}
	})
}

func TestRefusesATestThatSpellsItsName(t *testing.T) {
	t.Run("a string that spells the display name makes the masked input leak [verifies SP-1213~1]", func(t *testing.T) {
		_, err := java.New().Mask(sample(lines(
			"class SampleTest {",
			"    @Test",
			"    @DisplayName(\"grace period is free\")",
			"    void freePeriod() {",
			"        String label = \"grace period is free\";",
			"    }",
			"}",
		)), sampleTest(sampleClass, "grace period is free"))
		if !errors.Is(err, masking.ErrLeak) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a call that spells three words of the method name makes it leak [verifies SP-1213~1]", func(t *testing.T) {
		_, err := java.New().Mask(sample(lines(
			"class SampleTest {",
			"    @Test",
			"    void gracePeriodIsFree() {",
			"        audit.gracePeriodIsFree();",
			"    }",
			"}",
		)), sampleTest(sampleClass, "gracePeriodIsFree()"))
		if !errors.Is(err, masking.ErrLeak) {
			t.Fatalf("error = %v", err)
		}
	})
}
