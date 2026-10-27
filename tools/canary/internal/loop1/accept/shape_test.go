package accept_test

import (
	"reflect"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
)

func TestPromptTemplate(t *testing.T) {
	t.Run("the committed prompt of boxoffice follows the template [verifies SP-2551~1]", func(t *testing.T) {
		prompt := accept.Prompt("BOX-4", "specs/tickets.md", "free entry for children under 3", "boxoffice/prices.py")
		if !strings.Contains(string(committedConfig(t)), strings.ReplaceAll(prompt, `"`, `\"`)) {
			t.Fatalf("the committed prompt is not %q", prompt)
		}
		if err := accept.CheckPrompt(prompt, "BOX-4", "specs/tickets.md", "boxoffice/prices.py"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a prompt that departs from the template, or names other parts, is refused [verifies SP-2551~1]", func(t *testing.T) {
		good := accept.Prompt("BOX-4", "specs/tickets.md", "free entry", "boxoffice/prices.py")
		for name, prompt := range map[string]string{
			"another sentence":  "Do BOX-4 please.",
			"another criterion": strings.Replace(good, "BOX-4", "BOX-5", 1),
			"another spec":      strings.Replace(good, "specs/tickets.md", "specs/other.md", 1),
			"another source":    strings.Replace(good, "boxoffice/prices.py", "boxoffice/other.py", 1),
			"no statement":      accept.Prompt("BOX-4", "specs/tickets.md", " ", "boxoffice/prices.py"),
			"a multiline one":   accept.Prompt("BOX-4", "specs/tickets.md", "free\nentry", "boxoffice/prices.py"),
			"a long statement":  accept.Prompt("BOX-4", "specs/tickets.md", strings.Repeat("word ", 13), "boxoffice/prices.py"),
		} {
			if accept.CheckPrompt(prompt, "BOX-4", "specs/tickets.md", "boxoffice/prices.py") == nil {
				t.Errorf("%s was accepted", name)
			}
		}
	})

	t.Run("every prompt built from the template passes its own check [verifies SP-2551~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			words := rapid.SliceOfN(rapid.StringMatching(`[a-z0-9]{1,8}`), 1, 12).Draw(t, "words")
			prompt := accept.Prompt("ABC-12", "specs/a.md", strings.Join(words, " "), "pkg/mod.py")
			if err := accept.CheckPrompt(prompt, "ABC-12", "specs/a.md", "pkg/mod.py"); err != nil {
				t.Fatal(err)
			}
		})
	})
}

func TestOracleFileName(t *testing.T) {
	t.Run("a criterion ID becomes test_<lower case id with underscores>.py [verifies SP-2545~1]", func(t *testing.T) {
		for id, want := range map[string]string{"BOX-4": "test_box_4.py", "LATE-12": "test_late_12.py", "A1-3": "test_a1_3.py"} {
			if got := accept.OracleFileName(id); got != want {
				t.Errorf("%s: %s", id, got)
			}
		}
	})

	t.Run("two criteria never share a file [verifies SP-2545~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			id := rapid.StringMatching(`[A-Z][A-Z0-9]{0,5}-[0-9]{1,3}`)
			first, second := id.Draw(t, "first"), id.Draw(t, "second")
			if first != second && accept.OracleFileName(first) == accept.OracleFileName(second) {
				t.Fatalf("%s and %s", first, second)
			}
		})
	})
}

func TestParseExamples(t *testing.T) {
	spec := []byte("# Fares\n\n## Prices\n\n- **FARE-1~1**: WHEN a THE SYSTEM SHALL b.\n  - Example: fare_for(distance=5) => 2\n  - Example: fare_for(distance=7, zone=\"a,b\") => (2, 3)\n- **FARE-2~1**: WHEN c THE SYSTEM SHALL d.\n\n```\n- **FARE-3~1**: in a fence\n  - Example: fare_for(distance=1) => 1\n```\n### Notes\n  - Example: fare_for(distance=9) => 9\n")
	t.Run("the examples of each criterion are read from its block and fenced code holds none [verifies SP-2551~1]", func(t *testing.T) {
		got := accept.ParseExamples(spec)
		want := map[string][]accept.Example{
			"FARE-1": {{Operation: "fare_for", Arguments: "distance=5", Result: "2"}, {Operation: "fare_for", Arguments: `distance=7, zone="a,b"`, Result: "(2, 3)"}},
			"FARE-2": nil,
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("keyword arguments split at the commas outside brackets and quotes [verifies SP-2551~1]", func(t *testing.T) {
		arguments, err := accept.SplitArguments(`distance=7, zone="a,b", cuts=[1, 2]`)
		if err != nil || len(arguments) != 3 || arguments[1].Value != `"a,b"` || arguments[2].Name != "cuts" {
			t.Fatalf("%+v %v", arguments, err)
		}
		for _, bad := range []string{"5", "a=(1", `a="x`, "a=1, 2"} {
			if _, err := accept.SplitArguments(bad); err == nil {
				t.Errorf("%q was accepted", bad)
			}
		}
	})

	t.Run("arguments written by name always read back by name [verifies SP-2551~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			names := rapid.SliceOfN(rapid.StringMatching(`[a-z][a-z_]{0,6}`), 1, 4).Draw(t, "names")
			parts := make([]string, len(names))
			for index, name := range names {
				parts[index] = name + "=" + "[1, 2]"
			}
			got, err := accept.SplitArguments(strings.Join(parts, ", "))
			if err != nil || len(got) != len(names) {
				t.Fatalf("%v %v", got, err)
			}
			for index, argument := range got {
				if argument.Name != names[index] {
					t.Fatalf("%v", got)
				}
			}
		})
	})
}

func TestSignatureParameters(t *testing.T) {
	t.Run("the parameter names of a signature are listed in order, annotations and defaults dropped [verifies SP-2551~1]", func(t *testing.T) {
		got := accept.SignatureParameters("price(age, *, member: bool = False, **extra)")
		if !reflect.DeepEqual(got, []string{"age", "member", "extra"}) {
			t.Fatalf("%v", got)
		}
	})
}

func TestJUnit(t *testing.T) {
	data := []byte(`<testsuites><testsuite>
<testcase classname="test_box_4" name="test_a"/>
<testcase classname="test_box_4.TestX" name="test_b"><failure message="boom"/></testcase>
<testcase classname="" name="test_box_1"><error message="import"/></testcase>
<testcase classname="other" name="test_c"><skipped/></testcase>
</testsuite></testsuites>`)
	t.Run("a case fails on a failure or error child and belongs to the criterion whose file holds it [verifies SP-2550~1]", func(t *testing.T) {
		cases, err := accept.ParseJUnit(data, accept.ModulesOfCriteria([]string{"BOX-1", "BOX-4"}))
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, c := range cases {
			got[c.ID] = string(c.Status) + " " + c.Criterion
		}
		want := map[string]string{
			"test_box_4::test_a": "passed BOX-4", "test_box_4.TestX::test_b": "failed BOX-4", "test_box_1": "failed BOX-1", "other::test_c": "skipped ",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%v", got)
		}
	})
}
