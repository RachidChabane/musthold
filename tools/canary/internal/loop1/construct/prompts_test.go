package construct_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// words that no prompt, schema or finding sent to a model may hold.
var forbiddenInPrompts = []string{
	"study", "experiment", "gate", "mutant", "mutation", "oracle", "benchmark", "held-out", "measur",
	"planted", "weak", "deliberate", "adversar", "protocol", "hypothes", "loop-1", "grader", "judge",
}

func assertPlain(t *testing.T, what, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, word := range forbiddenInPrompts {
		if strings.Contains(lower, word) {
			t.Errorf("%s holds %q", what, word)
		}
	}
}

func TestPromptsAreOrdinaryInstructions(t *testing.T) {
	t.Run("no embedded template or schema mentions the study, a gate, mutants or what is measured [verifies SP-2542~1]", func(t *testing.T) {
		entries, err := os.ReadDir("prompts")
		if err != nil || len(entries) == 0 {
			t.Fatalf("%v", err)
		}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join("prompts", entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			assertPlain(t, entry.Name(), string(data))
		}
	})

	t.Run("the prompts of a whole run, repairs and findings included, hold none of those words [verifies SP-2542~1]", func(t *testing.T) {
		w := newThreeDomainWorld(t)
		_, _ = w.constructor.Run(t.Context())
		for _, request := range w.model.requests {
			assertPlain(t, request.Domain+" "+string(request.Role)+" "+string(request.Purpose), request.Prompt)
		}
	})
}

func TestAuthorPrompt(t *testing.T) {
	prompts, err := construct.NewPrompts()
	if err != nil {
		t.Fatal(err)
	}
	store := construct.Store{Repo: repoFromHere}
	example, err := store.ReadFiles(construct.BoxofficeFixture, "fixture/")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the prompt holds the domain, the shape rules with both properties, the prompt template and the example fixture files [verifies SP-2542~1]", func(t *testing.T) {
		prompt, err := prompts.Author(construct.AuthorInput{Domain: construct.DomainOrder()[0], ExamplePrompt: "EXAMPLE PROMPT", Example: example})
		if err != nil {
			t.Fatal(err)
		}
		wanted := []string{
			"late fees of a library loan by days overdue",
			"At least one implemented criterion has tests that check only the examples written inline in the spec",
			"The task criterion states at least one boundary or case beyond its inline examples",
			"4 to 6 criteria", "testpaths = tests", "pythonpath = .", "__pycache__/", "test-results/",
			"at least two violations", "at least one refactor",
			accept.PromptTemplate, "EXAMPLE PROMPT",
		}
		for _, file := range example {
			wanted = append(wanted, "--- file: "+file.Path, file.Content)
		}
		for _, text := range wanted {
			if !strings.Contains(prompt, text) {
				t.Errorf("the prompt lacks %q", text)
			}
		}
		assertPlain(t, "the author prompt", prompt)
	})

	t.Run("the example is the committed fixture, caches excluded [verifies SP-2542~1]", func(t *testing.T) {
		var paths []string
		for _, file := range example {
			paths = append(paths, file.Path)
		}
		want := []string{"fixture/.gitignore", "fixture/boxoffice/__init__.py", "fixture/boxoffice/prices.py", "fixture/pytest.ini", "fixture/specs/tickets.md", "fixture/tests/test_prices.py"}
		if strings.Join(paths, ",") != strings.Join(want, ",") {
			t.Fatalf("%v", paths)
		}
	})

	t.Run("the prompt template placeholders are filled by the author and checked by the harness [verifies SP-2543~1]", func(t *testing.T) {
		prompt := accept.Prompt("LATE-4", "specs/late.md", "a fee cap", "late/fees.py")
		if err := accept.CheckPrompt(prompt, "LATE-4", "specs/late.md", "late/fees.py"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestTestsPrompt(t *testing.T) {
	prompts, err := construct.NewPrompts()
	if err != nil {
		t.Fatal(err)
	}
	spec := "# Fares\n\n- **FARE-1~1**: WHEN a THE SYSTEM SHALL b.\n  - Example: fare_for(distance=5) => 2\n- **FARE-2~1**: WHEN c THE SYSTEM SHALL d.\n  - Example: fare_for(distance=6) => 3\n"
	iface := study.Interface{Module: "fare.zones", Operation: "fare_for", Signature: "fare_for(distance)"}

	t.Run("the prompt names one file per criterion, the interface, and what each file checks [verifies SP-2545~1]", func(t *testing.T) {
		prompt, err := prompts.Tests(construct.TestsInput{Spec: spec, Interface: iface})
		if err != nil {
			t.Fatal(err)
		}
		for _, wanted := range []string{
			"- FARE-1: test_fare_1.py", "- FARE-2: test_fare_2.py", "pytest.ini", "pythonpath = ..",
			"from fare.zones import fare_for", "Signature: fare_for(distance)", spec,
			"every inline example", "each side of every boundary", "bounded and deterministic", "at least two test functions", "standard library only",
		} {
			if !strings.Contains(prompt, wanted) {
				t.Errorf("the prompt lacks %q", wanted)
			}
		}
		assertPlain(t, "the tests prompt", prompt)
	})

	t.Run("a repair adds the writer's own previous files and the problems, nothing more [verifies SP-2547~1]", func(t *testing.T) {
		prompt, err := prompts.Tests(construct.TestsInput{
			Spec: spec, Interface: iface,
			Previous: []construct.File{{Path: "test_fare_1.py", Content: "PREVIOUS BODY"}}, Problems: []string{"A9 collection: too few tests"},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, wanted := range []string{"--- file: test_fare_1.py", "PREVIOUS BODY", "- A9 collection: too few tests"} {
			if !strings.Contains(prompt, wanted) {
				t.Errorf("the prompt lacks %q", wanted)
			}
		}
	})

	t.Run("a spec with no criterion is refused [verifies SP-2545~1]", func(t *testing.T) {
		if _, err := prompts.Tests(construct.TestsInput{Spec: "# Nothing\n", Interface: iface}); err == nil {
			t.Fatal("accepted")
		}
	})
}
