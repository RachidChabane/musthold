package java_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

const shapeSource = `package geo;

import java.util.List;

/** A shape. */
@Entity
public abstract class Shape<T> extends Base implements Comparable<Shape<T>> {
    public static final int SIDES = 4, CORNERS = 4;
    private int hidden = 1;
    protected final List<T> parts = new ArrayList<>();

    @Inject
    public Shape(int sides) { super(); }

    /** The area. */
    public abstract double area();

    @Override
    public int compareTo(Shape<T> other) { return 0; }

    private void audit() {}

    public static class Edge {
        public int length;
    }

    interface Visitor {
        void visit(Shape<?> shape);
    }
}
`

func signatureProject(testSource string) fstest.MapFS {
	return fstest.MapFS{
		sampleTestFile:                             {Data: []byte(testSource)},
		"src/main/java/shop/Cart.java":             {Data: []byte("package shop;\n\npublic class Cart {\n    public int size() { return 0; }\n}\n")},
		"src/main/java/shop/Mode.java":             {Data: []byte("package shop;\n\npublic enum Mode {\n    LOCAL, REMOTE;\n\n    public int rank() { return 1; }\n}\n")},
		"src/main/java/shop/Point.java":            {Data: []byte("package shop;\n\npublic record Point(int x, int y) {\n    public int sum() { return x + y; }\n}\n")},
		"src/main/java/geo/Shape.java":             {Data: []byte(shapeSource)},
		"src/main/java/geo/Rates.java":             {Data: []byte("package geo;\n\npublic class Rates {\n    public static int base() { return 1; }\n    public static int extra() { return 2; }\n    public static final int LIMIT = 3;\n}\n")},
		"src/main/java/geo/Kotlin.kt":              {Data: []byte("class Kotlin\n")},
		"src/test/java/shop/Support.java":          {Data: []byte("package shop;\n\nclass Support {\n    static int twice(int v) { return v * 2; }\n}\n")},
		"vendored/other/Outside.java":              {Data: []byte("package other;\n\npublic class Outside {}\n")},
		"service/src/main/java/svc/Clock.java":     {Data: []byte("package svc;\n\npublic class Clock {\n    public long now() { return 0; }\n}\n")},
		"service/src/test/java/svc/ClockTest.java": {Data: []byte("package svc;\n\nclass ClockTest {\n    @Test\n    void ticks() { new Clock().now(); }\n}\n")},
	}
}

func signaturesOf(t *testing.T, project fstest.MapFS) (signatures string, codeFiles []string) {
	t.Helper()
	masked := mustMask(t, project, sampleTest(sampleClass, "check()"))
	return masked.Signatures, masked.CodeFiles
}

func checkProject(imports []string, body string) fstest.MapFS {
	source := "package shop;\n\n" + lines(imports...) + "\nclass SampleTest {\n    @Test\n    void check() {\n        " + body + "\n    }\n}\n"
	return signatureProject(source)
}

func TestWritesSignatures(t *testing.T) {
	t.Run("classes are stubbed without bodies, annotations, comments, values or private members [verifies SP-1211~1]", func(t *testing.T) {
		got, files := signaturesOf(t, checkProject([]string{"import geo.Shape;"}, "Shape.SIDES.toString();"))
		assertEqual(t, got+"\n", lines(
			"public abstract class Shape<T> extends Base implements Comparable<Shape<T>> {",
			"    public static final int SIDES;",
			"    public static final int CORNERS;",
			"    protected final List<T> parts;",
			"    public Shape(int sides);",
			"    public abstract double area();",
			"    public int compareTo(Shape<T> other);",
			"    public static class Edge {",
			"        public int length;",
			"    }",
			"    interface Visitor {",
			"        void visit(Shape<?> shape);",
			"    }",
			"}",
		))
		if !slices.Equal(files, []string{"src/main/java/geo/Shape.java"}) {
			t.Errorf("files %v", files)
		}
	})

	t.Run("a static import writes the members it names and a static wildcard the whole class [verifies SP-1211~1]", func(t *testing.T) {
		got, _ := signaturesOf(t, checkProject([]string{"import static geo.Rates.base;"}, "base();"))
		assertEqual(t, got+"\n", lines(
			"public class Rates {",
			"    public static int base();",
			"}",
		))
		got, _ = signaturesOf(t, checkProject([]string{"import static geo.Rates.*;"}, "base(); extra();"))
		assertEqual(t, got+"\n", lines(
			"public class Rates {",
			"    public static int base();",
			"    public static int extra();",
			"    public static final int LIMIT;",
			"}",
		))
	})

	t.Run("a class of the package of the test needs no import, and an enum and a record are stubbed too [verifies SP-1211~1]", func(t *testing.T) {
		got, files := signaturesOf(t, checkProject(nil, "new Cart().size(); Mode.LOCAL.rank(); new Point(1, 2).sum(); Support.twice(1);"))
		for _, want := range []string{
			"public class Cart {\n    public int size();\n}",
			"public enum Mode {\n    LOCAL, REMOTE;\n    public int rank();\n}",
			"public record Point(int x, int y) {\n    public int sum();\n}",
			"class Support {\n    static int twice(int v);\n}",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("no %q in:\n%s", want, got)
			}
		}
		if len(files) != 4 {
			t.Errorf("files %v", files)
		}
	})

	t.Run("a wildcard import makes the names of its package resolvable [verifies SP-1211~1]", func(t *testing.T) {
		got, files := signaturesOf(t, checkProject([]string{"import geo.*;"}, "new Shape(1); Rates.base();"))
		if !strings.Contains(got, "public abstract class Shape<T>") || !strings.Contains(got, "public class Rates") {
			t.Fatalf("got:\n%s", got)
		}
		if !slices.Equal(files, []string{"src/main/java/geo/Rates.java", "src/main/java/geo/Shape.java"}) {
			t.Errorf("files %v", files)
		}
	})

	t.Run("a nested class is found through the class that holds it [verifies SP-1211~1]", func(t *testing.T) {
		got, _ := signaturesOf(t, checkProject([]string{"import geo.Shape.Edge;"}, "new Edge().length++;"))
		assertEqual(t, got+"\n", lines(
			"public static class Edge {",
			"    public int length;",
			"}",
		))
	})

	t.Run("the main sources of a module are found from its test sources [verifies SP-1211~1]", func(t *testing.T) {
		project := signatureProject("")
		got, files := func() (string, []string) {
			masked := mustMask(t, project, criterionOf("service/src/test/java/svc/ClockTest.java", "svc.ClockTest", "ticks()"))
			return masked.Signatures, masked.CodeFiles
		}()
		if got != "public class Clock {\n    public long now();\n}" || !slices.Equal(files, []string{"service/src/main/java/svc/Clock.java"}) {
			t.Fatalf("signatures %q files %v", got, files)
		}
	})

	t.Run("a class the project does not hold, a library class and a file of another language add nothing [verifies SP-1211~1]", func(t *testing.T) {
		got, files := signaturesOf(t, checkProject([]string{"import java.util.List;", "import other.Outside;", "import geo.Kotlin;"}, "List.of(); new Outside(); new Kotlin(); new Missing();"))
		if got != "" || len(files) != 0 {
			t.Fatalf("signatures %q files %v", got, files)
		}
	})
}
