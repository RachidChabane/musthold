package jvm

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

// Templates of the members the generated sources are made of. {n} makes the
// names unique, {op}, {cmp} and {num} are drawn.
var javaTemplates = []string{
	"static int calc{n}(int a, int b) {\n        if (a {cmp} {num}) {\n            return a {op} b;\n        }\n        return Math.min(a, {num}) {op} {num};\n    }",
	"static boolean flag{n}(boolean a, boolean b, int c) {\n        boolean x = a && !b || c {cmp} {num};\n        return x ? true : false;\n    }",
	"int total{n}(BigDecimal base) {\n        BigDecimal fee = new BigDecimal(\"{num}.50\");\n        return base.max(fee).intValue() {op} {num};\n    }",
	"static long span{n}(long a) {\n        Duration d = Duration.ofMinutes({num});\n        for (int i = 0; i < {num}; i++) {\n            a += i {op} 2;\n        }\n        return a + d.toMinutes();\n    }",
	"static void check{n}(int a) {\n        try {\n            if (a {cmp} {num}) {\n                throw new IllegalStateException(\"bad\");\n            }\n        } catch (RuntimeException e) {\n            throw new IllegalArgumentException(\"worse\");\n        }\n    }",
	"static String label{n}(int a) {\n        String s = \"v\" + a;\n        while (a > {num}) {\n            a--;\n        }\n        return s;\n    }",
	"static final int CAP{n} = {num};",
	"static final BigDecimal PRICE{n} = new BigDecimal(\"{num}.25\");",
	"static class In{n} {\n        int get(int a) {\n            return a {op} {num};\n        }\n    }",
	"static class Box{n} {\n        private int seed;\n\n        Box{n}(int a) {\n            super();\n            this.seed = a {op} {num};\n        }\n    }",
	"static char grade{n}(int a) {\n        if (a {cmp} {num}) {\n            return 'a';\n        } else {\n            return 'b';\n        }\n    }",
}

var kotlinTemplates = []string{
	"fun calc{n}(a: Int, b: Int): Int {\n    if (a {cmp} {num}) {\n        return a {op} b\n    }\n    return minOf(a, {num}) {op} {num}\n}",
	"fun flag{n}(a: Boolean, b: Boolean, c: Int): Boolean {\n    val x = a && !b || c {cmp} {num}\n    return if (x) true else false\n}",
	"fun total{n}(base: BigDecimal): Int {\n    val fee = BigDecimal(\"{num}.50\")\n    return base.max(fee).toInt() {op} {num}\n}",
	"fun span{n}(a: Long): Long {\n    var acc = a\n    for (i in 0..{num}) {\n        acc += i {op} 2\n    }\n    return acc + Duration.ofMinutes({num}).toMinutes()\n}",
	"fun check{n}(a: Int) {\n    try {\n        if (a {cmp} {num}) {\n            throw IllegalStateException(\"bad\")\n        }\n    } catch (e: RuntimeException) {\n        throw IllegalArgumentException(\"worse\")\n    }\n}",
	"fun label{n}(a: Int): String {\n    var k = a\n    val s = \"v\" + k\n    while (k > {num}) {\n        k--\n    }\n    return s\n}",
	"const val CAP{n} = {num}",
	"val PRICE{n} = BigDecimal(\"{num}.25\")",
	"object Obj{n} {\n    val limit = {num}\n    fun get(a: Int): Int {\n        return a {op} {num}\n    }\n}",
	"class Crate{n} {\n    val size = {num}\n    fun grow(a: Int): Double {\n        return a * 1.5\n    }\n    companion object {\n        const val MAX = {num}\n        fun make(): Crate{n} = Crate{n}()\n    }\n}",
}

const (
	javaHeader   = "import java.math.BigDecimal;\nimport java.time.Duration;\n\nclass T {\n    "
	javaFooter   = "\n}\n"
	kotlinHeader = "import java.math.BigDecimal\nimport java.time.Duration\n\n"
	memberJoint  = "\n\n    "
	topJoint     = "\n\n"
)

var (
	arithmeticDraws = []string{"+", "-", "*", "/", "%"}
	comparisonDraws = []string{"<", "<=", ">", ">=", "==", "!="}
	numberDraws     = []string{"0", "1", "7", "15", "100"}
)

// generatedSource draws a source of the language of path from the templates.
func generatedSource(t *rapid.T, path string) string {
	templates, header, footer, joint := javaTemplates, javaHeader, javaFooter, memberJoint
	if path == kotlinPath {
		templates, header, footer, joint = kotlinTemplates, kotlinHeader, "", topJoint
	}
	chosen := rapid.SliceOfN(rapid.IntRange(0, len(templates)-1), 1, 3).Draw(t, "templates")
	members := make([]string, 0, len(chosen))
	for position, index := range chosen {
		replacer := strings.NewReplacer(
			"{n}", string(rune('A'+position)),
			"{op}", rapid.SampledFrom(arithmeticDraws).Draw(t, "op"),
			"{cmp}", rapid.SampledFrom(comparisonDraws).Draw(t, "cmp"),
			"{num}", rapid.SampledFrom(numberDraws).Draw(t, "num"),
		)
		members = append(members, replacer.Replace(templates[index]))
	}
	return header + strings.Join(members, joint) + footer + "\n"
}

func TestSplicingProperties(t *testing.T) {
	engine := New()
	for _, path := range []string{javaPath, kotlinPath} {
		t.Run(path+" every operator mutant splices back with the post-splice invariant [verifies SP-1105~1]", func(t *testing.T) {
			rapid.Check(t, func(t *rapid.T) {
				source := generatedSource(t, path)
				candidates, err := engine.Candidates(sourceFile(path, source))
				if err != nil {
					t.Fatalf("source %q: %v", source, err)
				}
				for _, candidate := range candidates {
					edit := candidate.Edit
					spliced, err := splice(path, []byte(source), edit)
					if err != nil {
						t.Fatalf("%s %s: %v\n%s", edit.Target, edit.Qualname, err, source)
					}
					if want := source[:candidate.Replaces.Start] + edit.NewSource + source[candidate.Replaces.End:]; string(spliced) != want {
						t.Fatalf("splice differs from the replacement of the reported span")
					}
				}
			})
		})

		t.Run(path+" the screen keeps mutants that parse and they splice into a parsable file [verifies SP-1105~1]", func(t *testing.T) {
			rapid.Check(t, func(t *rapid.T) {
				source := generatedSource(t, path)
				candidates, err := engine.Candidates(sourceFile(path, source))
				if err != nil {
					t.Fatal(err)
				}
				kept, _, err := operator.Screen(candidates, engine)
				if err != nil {
					t.Fatalf("the screen refused the unit of a source it enumerated: %v\n%s", err, source)
				}
				for _, mutant := range kept {
					edit := mutant.EditSet.Edits[0]
					spliced, err := splice(path, []byte(source), edit)
					if err != nil {
						t.Fatalf("%s: %v", edit.Qualname, err)
					}
					if _, err := engine.CallGraph(sourceFile(path, string(spliced))); err != nil {
						t.Fatalf("a kept mutant gives an unparsable file: %v\n%s", err, spliced)
					}
				}
			})
		})

		t.Run(path+" every sentinel splices back, parses and leaves the units alone [verifies SP-1105~1]", func(t *testing.T) {
			rapid.Check(t, func(t *rapid.T) {
				source := generatedSource(t, path)
				seed := operator.Seed(rapid.Uint64().Draw(t, "seed"))
				sentinels, err := engine.SentinelCandidates(sourceFile(path, source), seed)
				if err != nil {
					t.Fatal(err)
				}
				for _, sentinel := range sentinels {
					edit := sentinel.Edit
					spliced, err := splice(path, []byte(source), edit)
					if err != nil {
						t.Fatalf("%s %s: %v\n%s", edit.Target, edit.Qualname, err, source)
					}
					if _, err := engine.CallGraph(sourceFile(path, string(spliced))); err != nil {
						t.Fatalf("sentinel %s gives an unparsable file: %v\n%s", sentinel.Kind, err, spliced)
					}
					if edit.Target == mutation.TargetModuleAddition {
						continue
					}
					before, after := unitNames(t, path, source), unitNames(t, path, string(spliced))
					if strings.Join(before, ",") != strings.Join(after, ",") {
						t.Fatalf("units changed: %v -> %v", before, after)
					}
				}
			})
		})
	}
}

// unitNames lists the targets and qualnames of the units of a source, for a
// check inside a property.
func unitNames(t *rapid.T, path, source string) []string {
	parsedFile, err := parseChecked(path, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	defer parsedFile.release()
	var names []string
	for _, u := range parsedFile.units() {
		names = append(names, string(u.target)+":"+u.qualname)
	}
	return names
}
