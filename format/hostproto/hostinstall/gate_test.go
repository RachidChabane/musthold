package hostinstall_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto/hostinstall"
)

// layouts are projects of each ecosystem, as the files their layout shows, and the
// roots a proof may name as code in each.
var layouts = map[string]struct {
	files []string
	roots []string
}{
	"a flat pytest project":    {[]string{"pytest.ini", "parcel/__init__.py", "parcel/rates.py", "tests/__init__.py", "tests/test_rates.py", "specs/parcel.md"}, []string{"parcel"}},
	"a pytest project in src":  {[]string{"pyproject.toml", "src/parcel/__init__.py", "tests/test_rates.py"}, []string{"src"}},
	"a Go module":              {[]string{"go.mod", "rates.go", "rates_test.go"}, []string{"**/*.go"}},
	"a Jest project":           {[]string{"package.json", "jest.config.js", "src/rates.ts", "test/rates.test.ts"}, []string{"src"}},
	"a Maven project":          {[]string{"pom.xml", "src/main/java/p/Rates.java", "src/test/java/p/RatesTest.java"}, []string{"src/main"}},
	"a Gradle project":         {[]string{"build.gradle.kts", "src/main/kotlin/p/Rates.kt"}, []string{"src/main"}},
	"a polyglot project":       {[]string{"go.mod", "package.json", "src/index.ts"}, []string{"**/*.go", "src"}},
	"a project with no layout": {[]string{"README.md", "notes/ideas.txt"}, nil},
}

func layoutFiles(t *testing.T, paths []string) *memFiles {
	t.Helper()
	files := repository(t)
	for _, path := range paths {
		files.put(t, path, "x")
	}
	return files
}

func TestSourceRoots(t *testing.T) {
	for name, l := range layouts {
		t.Run("the source roots of "+name+" are read from its files [verifies SP-1603~1]", func(t *testing.T) {
			if got := hostinstall.SourceRoots(layoutFiles(t, l.files)); !slices.Equal(got, l.roots) {
				t.Fatalf("roots = %q, want %q", got, l.roots)
			}
		})
	}

	t.Run("a top-level directory that is no package, a test directory or a hidden one is no source root [verifies SP-1603~1]", func(t *testing.T) {
		files := layoutFiles(t, []string{"pytest.ini", "tests/__init__.py", ".venv/__init__.py", "_private/__init__.py", "docs/__init__.py", "scripts/run.py", "alpha/__init__.py", "beta/__init__.py"})
		if got := hostinstall.SourceRoots(files); !slices.Equal(got, []string{"alpha", "beta"}) {
			t.Fatalf("roots = %q", got)
		}
	})
}

func TestEnableWritesTheLocalLoopSettings(t *testing.T) {
	eachModule(t, "enable sets proofs.trust to local and lists the source roots of the project when the gate configuration sets neither", enableTag+" [verifies SP-1603~1]", func(t *testing.T, s sample) {
		if !s.module.Required {
			t.Skip("applies to a proof Shallnot requires")
		}
		files := layoutFiles(t, layouts["a flat pytest project"].files)
		files.put(t, shallnotPath, traceabilityOnly)
		apply(t, files)(s.module.BuildEnable(files))
		want := traceabilityOnly + "proofs:\n  require:\n    - " + s.kind() + "\n  trust: local\n  code:\n    - parcel\n"
		if got := files.text(shallnotPath); got != want {
			t.Fatalf("shallnot.yaml = %q, want %q", got, want)
		}
	})

	eachModule(t, "enable keeps the trust and the code the gate configuration sets and every comment around them", enableTag+" [verifies SP-1603~1]", func(t *testing.T, s sample) {
		if !s.module.Required {
			t.Skip("applies to a proof Shallnot requires")
		}
		before := "version: 1 # keep\nproofs:\n  trust: signed # CI\n  code:\n    - lib # mine\ntests:\n  - tests\n"
		files := layoutFiles(t, layouts["a flat pytest project"].files)
		files.put(t, shallnotPath, before)
		apply(t, files)(s.module.BuildEnable(files))
		want := "version: 1 # keep\nproofs:\n  trust: signed # CI\n  code:\n    - lib # mine\n  require:\n    - " + s.kind() + "\ntests:\n  - tests\n"
		if got := files.text(shallnotPath); got != want {
			t.Fatalf("shallnot.yaml = %q, want %q", got, want)
		}
	})

	eachModule(t, "enable fills an empty code list and a flow one and keeps the layout of the block", enableTag+" [verifies SP-1603~1]", func(t *testing.T, s sample) {
		if !s.module.Required {
			t.Skip("applies to a proof Shallnot requires")
		}
		for name, before := range map[string]string{
			"null":   "proofs:\n  code:\n  trust: local\n",
			"empty":  "proofs:\n  code: []\n  trust: local\n",
			"absent": "proofs:\n  trust: local\n",
		} {
			files := layoutFiles(t, layouts["a Go module"].files)
			files.put(t, shallnotPath, before)
			apply(t, files)(s.module.BuildEnable(files))
			got := files.text(shallnotPath)
			if !strings.Contains(got, "**/*.go") || !strings.Contains(got, "- "+s.kind()) || strings.Count(got, "trust:") != 1 {
				t.Errorf("%s: shallnot.yaml = %q", name, got)
			}
		}
	})

	eachModule(t, "a second enable, from another module of the same project, changes neither the trust nor the code", enableTag+" [verifies SP-1603~1]", func(t *testing.T, s sample) {
		if !s.module.Required {
			t.Skip("applies to a proof Shallnot requires")
		}
		files := layoutFiles(t, layouts["a polyglot project"].files)
		files.put(t, shallnotPath, traceabilityOnly)
		apply(t, files)(s.module.BuildEnable(files))
		first := files.text(shallnotPath)
		for _, other := range samples() {
			if other.module.Required {
				apply(t, files)(other.module.BuildEnable(files))
			}
		}
		after := files.text(shallnotPath)
		if strings.Count(after, "trust:") != 1 || strings.Count(after, "**/*.go") != 1 || strings.Count(after, "- src") != 1 || !strings.HasPrefix(after, first[:strings.Index(first, "  require:")]) {
			t.Fatalf("first = %q, after = %q", first, after)
		}
	})

	eachModule(t, "disable leaves the trust and the code, which the other modules rely on", enableTag+" [verifies SP-1603~1]", func(t *testing.T, s sample) {
		if !s.module.Required {
			t.Skip("applies to a proof Shallnot requires")
		}
		files := layoutFiles(t, layouts["a flat pytest project"].files)
		files.put(t, shallnotPath, traceabilityOnly)
		apply(t, files)(s.module.BuildEnable(files))
		apply(t, files)(s.module.BuildDisable(files))
		want := traceabilityOnly + "proofs:\n  trust: local\n  code:\n    - parcel\n"
		if got := files.text(shallnotPath); got != want {
			t.Fatalf("shallnot.yaml = %q, want %q", got, want)
		}
	})

	eachModule(t, "enable names the missing source roots in its next steps only when none is found", enableTag+" [verifies SP-1603~1]", func(t *testing.T, s sample) {
		if !s.module.Required {
			t.Skip("applies to a proof Shallnot requires")
		}
		withLayout := layoutFiles(t, layouts["a Go module"].files)
		plan, err := s.module.BuildEnable(withLayout)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.Join(plan.NextSteps, "\n"), "no source root") {
			t.Errorf("next steps = %q", plan.NextSteps)
		}
	})
}

func TestInitWritesTheOperationsExample(t *testing.T) {
	eachModule(t, "init ends a new configuration with the commented example of the operations block", initTag+" [verifies SP-1604~1]", func(t *testing.T, s sample) {
		files := repository(t)
		apply(t, files)(initPlan(s, files))
		got := files.text(configPath)
		if !strings.HasSuffix(got, hostinstall.OperationsExample) || !strings.Contains(got, "# operations:\n#   line_total:") {
			t.Fatalf("config = %q", got)
		}
		for _, line := range strings.Split(strings.TrimSpace(hostinstall.OperationsExample), "\n") {
			if !strings.HasPrefix(line, "#") {
				t.Errorf("the example line %q is not a comment", line)
			}
		}
	})

	eachModule(t, "init adds the example to a configuration another module wrote, once", initTag+" [verifies SP-1604~1]", func(t *testing.T, s sample) {
		files := repository(t)
		files.put(t, configPath, "proofs:\n  "+otherProof+": false\n")
		apply(t, files)(initPlan(s, files))
		first := files.text(configPath)
		if strings.Count(first, hostinstall.OperationsExample) != 1 || !strings.HasPrefix(first, "proofs:\n  "+otherProof+": false\n  "+s.kind()+": false\n\n#") {
			t.Fatalf("config = %q", first)
		}
		apply(t, files)(initPlan(s, files))
		if again := files.text(configPath); again != first {
			t.Fatalf("a second init changed the configuration: %q", again)
		}
	})

	eachModule(t, "init leaves a configuration that binds operations without the example", initTag+" [verifies SP-1604~1]", func(t *testing.T, s sample) {
		before := "proofs:\n  " + s.kind() + ": false\noperations:\n  line_total:\n    python: parcel.rates:line_total\n"
		files := repository(t)
		files.put(t, configPath, before)
		apply(t, files)(initPlan(s, files))
		if got := files.text(configPath); got != before {
			t.Fatalf("config = %q", got)
		}
	})

	eachModule(t, "init keeps the local state of the proofs out of version control, the pending files included", initTag+" [verifies SP-1604~1]", func(t *testing.T, s sample) {
		files := repository(t)
		apply(t, files)(initPlan(s, files))
		ignored := files.text(gitignorePath)
		for _, want := range []string{".musthold/cache/", ".musthold/verified/", ".musthold/pending/"} {
			if !slices.Contains(strings.Split(ignored, "\n"), want) {
				t.Errorf(".gitignore = %q lacks %s", ignored, want)
			}
		}
	})
}
