package kotlin_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

const shapeSource = `package geo

import java.util.List

/** A shape. */
@Entity
abstract class Shape<T>(val sides: Int, private val secret: String) : Base(), Comparable<Shape<T>> {
    val parts: List<T> = mutableListOf()
    private var hidden = 1
    var label: String = "x"
        get() = field.trim()

    @Inject
    constructor(sides: Int) : this(sides, "")

    /** The area. */
    abstract fun area(): Double

    @Override
    override fun compareTo(other: Shape<T>): Int = 0

    private fun audit() {}

    class Edge(val length: Int)

    interface Visitor {
        fun visit(shape: Shape<*>)
    }

    companion object {
        const val SIDES = 4
        fun unit(): Shape<Int>? = null
    }
}
`

func signatureProject(testSource string) fstest.MapFS {
	return fstest.MapFS{
		sampleTestFile:                         {Data: []byte(testSource)},
		"src/main/kotlin/shop/Cart.kt":         {Data: []byte("package shop\n\nclass Cart {\n    fun size(): Int = 0\n}\n")},
		"src/main/kotlin/shop/Mode.kt":         {Data: []byte("package shop\n\nenum class Mode {\n    LOCAL, REMOTE;\n\n    fun rank(): Int = 1\n}\n")},
		"src/main/kotlin/shop/Models.kt":       {Data: []byte("package shop\n\ndata class Point(val x: Int, val y: Int) {\n    fun sum() = x + y\n}\n\nfun twice(v: Int): Int = v * 2\n\nval TOP: Int = 3\n\nprivate fun hidden() {}\n")},
		"src/main/kotlin/geo/Shape.kt":         {Data: []byte(shapeSource)},
		"src/main/kotlin/geo/Rates.kt":         {Data: []byte("package geo\n\nobject Rates {\n    fun base(): Int = 1\n    fun extra(): Int = 2\n    const val LIMIT = 3\n}\n\nfun levy(amount: Int): Int = amount\n")},
		"src/main/java/geo/Legacy.java":        {Data: []byte("package geo;\n\npublic class Legacy {}\n")},
		"src/test/kotlin/shop/Support.kt":      {Data: []byte("package shop\n\nclass Support {\n    fun help(): Int = 1\n}\n")},
		"vendored/other/Outside.kt":            {Data: []byte("package other\n\nclass Outside\n")},
		"service/src/main/kotlin/svc/Clock.kt": {Data: []byte("package svc\n\nclass Clock {\n    fun now(): Long = 0\n}\n")},
		"service/src/test/kotlin/svc/Ticks.kt": {Data: []byte("package svc\n\nclass Ticks {\n    @Test\n    fun ticks() { Clock().now() }\n}\n")},
	}
}

func signaturesOf(t *testing.T, project fstest.MapFS) (signatures string, codeFiles []string) {
	t.Helper()
	masked := mustMask(t, project, sampleTest(sampleClass, "check()"))
	return masked.Signatures, masked.CodeFiles
}

func checkProject(imports []string, body string) fstest.MapFS {
	source := "package shop\n\n" + lines(imports...) + "\nclass SampleTest {\n    @Test\n    fun check() {\n        " + body + "\n    }\n}\n"
	return signatureProject(source)
}

func TestWritesSignatures(t *testing.T) {
	t.Run("classes are stubbed without bodies, annotations, comments, values or private members [verifies SP-1221~1]", func(t *testing.T) {
		got, files := signaturesOf(t, checkProject([]string{"import geo.Shape"}, "Shape.SIDES.toString()"))
		assertEqual(t, got+"\n", lines(
			"abstract class Shape<T>(val sides: Int, private val secret: String) : Base(), Comparable<Shape<T>> {",
			"    val parts: List<T>",
			"    var label: String",
			"    abstract fun area(): Double",
			"    override fun compareTo(other: Shape<T>): Int",
			"    class Edge(val length: Int)",
			"    interface Visitor {",
			"        fun visit(shape: Shape<*>)",
			"    }",
			"    companion object {",
			"        const val SIDES",
			"        fun unit(): Shape<Int>?",
			"    }",
			"}",
		))
		if !slices.Equal(files, []string{"src/main/kotlin/geo/Shape.kt"}) {
			t.Errorf("files %v", files)
		}
	})

	t.Run("an import of a member of an object writes the member alone [verifies SP-1221~1]", func(t *testing.T) {
		got, _ := signaturesOf(t, checkProject([]string{"import geo.Rates.base"}, "base()"))
		assertEqual(t, got+"\n", lines(
			"object Rates {",
			"    fun base(): Int",
			"}",
		))
	})

	t.Run("a top-level function is found in any file of its package, and a private one is not a signature [verifies SP-1221~1]", func(t *testing.T) {
		got, files := signaturesOf(t, checkProject([]string{"import geo.levy", "import shop.hidden"}, "levy(1); hidden()"))
		if got != "fun levy(amount: Int): Int" {
			t.Errorf("signatures %q", got)
		}
		if !slices.Equal(files, []string{"src/main/kotlin/geo/Rates.kt"}) {
			t.Errorf("files %v", files)
		}
	})

	t.Run("a name of the package of the test needs no import, and an enum, a data class and a top-level declaration are stubbed too [verifies SP-1221~1]", func(t *testing.T) {
		got, files := signaturesOf(t, checkProject(nil, "Cart().size(); Mode.LOCAL.rank(); Point(1, 2).sum(); twice(TOP); Support().help()"))
		for _, want := range []string{
			"class Cart {\n    fun size(): Int\n}",
			"enum class Mode {\n    LOCAL, REMOTE\n    fun rank(): Int\n}",
			"data class Point(val x: Int, val y: Int) {\n    fun sum()\n}",
			"fun twice(v: Int): Int",
			"val TOP: Int",
			"class Support {\n    fun help(): Int\n}",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("no %q in:\n%s", want, got)
			}
		}
		if len(files) != 4 {
			t.Errorf("files %v", files)
		}
	})

	t.Run("a wildcard import makes the names of its package resolvable [verifies SP-1221~1]", func(t *testing.T) {
		got, files := signaturesOf(t, checkProject([]string{"import geo.*"}, "Shape(1); Rates.base(); levy(1)"))
		if !strings.Contains(got, "abstract class Shape<T>") || !strings.Contains(got, "object Rates") || !strings.Contains(got, "fun levy(amount: Int): Int") {
			t.Fatalf("got:\n%s", got)
		}
		if !slices.Equal(files, []string{"src/main/kotlin/geo/Rates.kt", "src/main/kotlin/geo/Shape.kt"}) {
			t.Errorf("files %v", files)
		}
	})

	t.Run("a type nested in a class is found through the class that holds it [verifies SP-1221~1]", func(t *testing.T) {
		got, _ := signaturesOf(t, checkProject([]string{"import geo.Shape.Edge"}, "Edge(1).length"))
		assertEqual(t, got+"\n", "class Edge(val length: Int)\n")
	})

	t.Run("the main sources of a module are found from its test sources [verifies SP-1221~1]", func(t *testing.T) {
		masked := mustMask(t, signatureProject(""), criterionOf("service/src/test/kotlin/svc/Ticks.kt", "svc.Ticks", "ticks()"))
		if masked.Signatures != "class Clock {\n    fun now(): Long\n}" || !slices.Equal(masked.CodeFiles, []string{"service/src/main/kotlin/svc/Clock.kt"}) {
			t.Fatalf("signatures %q files %v", masked.Signatures, masked.CodeFiles)
		}
	})

	t.Run("a declaration the project does not hold, a library one and a file of another language add nothing [verifies SP-1221~1]", func(t *testing.T) {
		got, files := signaturesOf(t, checkProject([]string{"import java.util.List", "import other.Outside", "import geo.Legacy"}, "List.of(); Outside(); Legacy(); Missing()"))
		if got != "" || len(files) != 0 {
			t.Fatalf("signatures %q files %v", got, files)
		}
	})
}
