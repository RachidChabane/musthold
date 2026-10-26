package prompt_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
	"github.com/RachidChabane/musthold/format"
)

const (
	verifiesSP2162 = " [verifies SP-2162~1]"
	verifiesSP2163 = " [verifies SP-2163~1]"
	verifiesSP2164 = " [verifies SP-2164~1]"
	verifiesSP2165 = " [verifies SP-2165~1]"

	readmeText      = "The parser returns an empty list for an empty file."
	descriptionText = "Fix: the parser no longer raises on a blank line."
	diffHunk        = "+    if line.strip() == '': continue  # hunk-marker"
)

// compileResponseSchema compiles the response schema of a built prompt.
func compileResponseSchema(t testing.TB, built prompt.Built) *jsonschema.Schema {
	t.Helper()
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(built.Schema))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	const resource = "memory:///response.json"
	if err := compiler.AddResource(resource, document); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(resource)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

// fataler is what a testing.T and a rapid.T share.
type fataler interface {
	Helper()
	Fatal(args ...any)
}

func schemaAccepts(t fataler, schema *jsonschema.Schema, output string) bool {
	t.Helper()
	instance, err := jsonschema.UnmarshalJSON(strings.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}
	return schema.Validate(instance) == nil
}

func promisesInput() prompt.PromisesInput {
	return prompt.PromisesInput{
		Sources: []prompt.PromiseSource{
			{Name: format.ScanSourceReadme, Text: readmeText},
			{Name: format.ScanSourceDescription, Text: descriptionText},
			{Name: format.ScanSourceIssue(12), Text: "Blank lines must be skipped."},
		},
		ChangedPaths: []string{"src/parser.py"},
		Signatures:   []string{"def parse(text: str) -> list[str]: ..."},
	}
}

func TestBuildPromises(t *testing.T) {
	input := promisesInput()
	built := prompt.BuildPromises(input)
	t.Run("every source is shown under its name with its exact text"+verifiesSP2162, func(t *testing.T) {
		for _, source := range input.Sources {
			if !strings.Contains(built.Prompt.User, "Source "+source.Name+":\n<<<\n"+source.Text+"\n>>>") {
				t.Errorf("source %s is missing or altered", source.Name)
			}
		}
	})
	t.Run("a change shows its changed paths and public signatures"+verifiesSP2162, func(t *testing.T) {
		for _, want := range append(slices.Clone(input.ChangedPaths), input.Signatures...) {
			if !strings.Contains(built.Prompt.User, want) {
				t.Errorf("%q is missing", want)
			}
		}
	})
	t.Run("a prompt without a change shows no list of paths or signatures"+verifiesSP2162, func(t *testing.T) {
		wholeRepository := prompt.BuildPromises(prompt.PromisesInput{Sources: input.Sources})
		if strings.Contains(wholeRepository.Prompt.User, "Files the change touches") || strings.Contains(wholeRepository.Prompt.User, "Public signatures") {
			t.Errorf("prompt %q", wholeRepository.Prompt.User)
		}
	})
	t.Run("the input has no member that carries a hunk or a body, so none reaches the prompt"+verifiesSP2162, func(t *testing.T) {
		if strings.Contains(built.Prompt.System+built.Prompt.User, diffHunk) {
			t.Error("the prompt carries a hunk")
		}
	})
	t.Run("the response schema is the promises schema and the digest ignores the input"+verifiesSP2162, func(t *testing.T) {
		if !strings.Contains(string(built.Schema), `"promises"`) {
			t.Errorf("schema %s", built.Schema)
		}
		if prompt.BuildPromises(prompt.PromisesInput{}).Prompt.TemplateDigest != built.Prompt.TemplateDigest {
			t.Error("the digest depends on the input")
		}
	})
}

func TestPromisesSchema(t *testing.T) {
	schema := compileResponseSchema(t, prompt.BuildPromises(prompt.PromisesInput{}))
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{"a promise with a quote and a source", `{"promises":[{"quote":"returns an empty list","source":"README.md"}]}`, true},
		{"an empty list", `{"promises":[]}`, true},
		{"a closing issue source", `{"promises":[{"quote":"q","source":"issue #12"}]}`, true},
		{"the description source", `{"promises":[{"quote":"q","source":"description"}]}`, true},
		{"a missing quote", `{"promises":[{"source":"README.md"}]}`, false},
		{"an empty quote", `{"promises":[{"quote":"","source":"README.md"}]}`, false},
		{"a missing source", `{"promises":[{"quote":"q"}]}`, false},
		{"a source outside the closed set", `{"promises":[{"quote":"q","source":"CHANGELOG.md"}]}`, false},
		{"an issue number with a leading zero", `{"promises":[{"quote":"q","source":"issue #012"}]}`, false},
		{"a paraphrase member", `{"promises":[{"quote":"q","source":"README.md","paraphrase":"p"}]}`, false},
		{"an extra top-level member", `{"promises":[],"notes":"n"}`, false},
		{"no promises member", `{}`, false},
		{"more promises than the limit", `{"promises":[` + strings.TrimSuffix(strings.Repeat(`{"quote":"q","source":"README.md"},`, prompt.MaxExtractedPromises+1), ",") + `]}`, false},
	}
	for _, c := range cases {
		t.Run(c.name+verifiesSP2162, func(t *testing.T) {
			if got := schemaAccepts(t, schema, c.output); got != c.want {
				t.Errorf("schema accepts = %v, want %v", got, c.want)
			}
		})
	}
	t.Run("the schema source pattern accepts exactly the names of the format"+verifiesSP2162, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			name := rapid.OneOf(
				rapid.Just(format.ScanSourceReadme),
				rapid.Just(format.ScanSourceDescription),
				rapid.Custom(func(t *rapid.T) string { return format.ScanSourceIssue(rapid.IntRange(-3, 100000).Draw(t, "number")) }),
				rapid.StringMatching(`[A-Za-z. #0-9]{0,12}`),
			).Draw(t, "name")
			quoted, err := json.Marshal(name)
			if err != nil {
				t.Fatal(err)
			}
			output := fmt.Sprintf(`{"promises":[{"quote":"q","source":%s}]}`, quoted)
			if got, want := schemaAccepts(t, schema, output), format.IsScanSourceName(name); got != want {
				t.Fatalf("%q: schema accepts = %v, format accepts = %v", name, got, want)
			}
		})
	})
}

func TestParsePromises(t *testing.T) {
	names := []string{format.ScanSourceReadme, format.ScanSourceDescription}
	t.Run("the promises come back in order with their quote and source"+verifiesSP2163, func(t *testing.T) {
		output := `{"promises":[{"quote":"b","source":"description"},{"quote":"a","source":"README.md"}]}`
		got, err := prompt.ParsePromises([]byte(output), names)
		want := []prompt.ExtractedPromise{{Quote: "b", Source: "description"}, {Quote: "a", Source: "README.md"}}
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("got %+v, %v", got, err)
		}
	})
	t.Run("an empty list is an answer"+verifiesSP2163, func(t *testing.T) {
		got, err := prompt.ParsePromises([]byte(`{"promises":[]}`), names)
		if err != nil || len(got) != 0 {
			t.Errorf("got %+v, %v", got, err)
		}
	})
	t.Run("a source the request did not show is refused"+verifiesSP2163, func(t *testing.T) {
		_, err := prompt.ParsePromises([]byte(`{"promises":[{"quote":"a","source":"issue #4"}]}`), names)
		if !errors.Is(err, prompt.ErrUnknownPromiseSource) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("an empty quote is refused"+verifiesSP2163, func(t *testing.T) {
		_, err := prompt.ParsePromises([]byte(`{"promises":[{"quote":"","source":"README.md"}]}`), names)
		if !errors.Is(err, prompt.ErrEmptyPromiseQuote) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("a list longer than the limit is refused"+verifiesSP2163, func(t *testing.T) {
		item := `{"quote":"a","source":"README.md"}`
		output := `{"promises":[` + strings.TrimSuffix(strings.Repeat(item+",", prompt.MaxExtractedPromises+1), ",") + `]}`
		if _, err := prompt.ParsePromises([]byte(output), names); !errors.Is(err, prompt.ErrTooManyPromises) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("a response that is not JSON is refused"+verifiesSP2163, func(t *testing.T) {
		if _, err := prompt.ParsePromises([]byte(brokenJSON), names); err == nil {
			t.Error("accepted")
		}
	})
}

func labelsInput() prompt.LabelsInput {
	return prompt.LabelsInput{
		Promises: []prompt.LabelPromise{
			{ID: prompt.PromiseID(0), Quote: "returns an empty list"},
			{ID: prompt.PromiseID(1), Quote: "never raises"},
		},
		Functions: []prompt.LabelFunction{
			{ID: prompt.FunctionID(0), Path: "src/parser.py", Signature: "def parse(text)", Code: "def parse(text):\n    return []\n"},
			{ID: prompt.FunctionID(1), Path: "src/lexer.py", Signature: "def lex(text)", Code: "def lex(text):\n    return text.split()\n"},
		},
	}
}

func TestBuildLabels(t *testing.T) {
	input := labelsInput()
	built := prompt.BuildLabels(input)
	t.Run("every promise is shown under its id with its quote"+verifiesSP2164, func(t *testing.T) {
		for _, promise := range input.Promises {
			if !strings.Contains(built.Prompt.User, promise.ID+": "+fmt.Sprintf("%q", promise.Quote)) {
				t.Errorf("promise %s is missing", promise.ID)
			}
		}
	})
	t.Run("every function is shown under its id with its path, signature and code"+verifiesSP2164, func(t *testing.T) {
		for _, function := range input.Functions {
			for _, want := range []string{"Function " + function.ID, function.Path, function.Signature, strings.TrimSpace(function.Code)} {
				if !strings.Contains(built.Prompt.User, want) {
					t.Errorf("%q is missing", want)
				}
			}
		}
	})
	t.Run("the ids are p1, p2 and f1, f2 and so on"+verifiesSP2164, func(t *testing.T) {
		if prompt.PromiseID(0) != "p1" || prompt.PromiseID(1) != "p2" || prompt.FunctionID(0) != "f1" || prompt.FunctionID(2) != "f3" {
			t.Errorf("got %s %s %s %s", prompt.PromiseID(0), prompt.PromiseID(1), prompt.FunctionID(0), prompt.FunctionID(2))
		}
	})
	t.Run("the response schema is the labels schema and the digest ignores the input"+verifiesSP2164, func(t *testing.T) {
		if !strings.Contains(string(built.Schema), `"labels"`) {
			t.Errorf("schema %s", built.Schema)
		}
		if prompt.BuildLabels(prompt.LabelsInput{}).Prompt.TemplateDigest != built.Prompt.TemplateDigest {
			t.Error("the digest depends on the input")
		}
	})
}

func TestLabelsSchema(t *testing.T) {
	schema := compileResponseSchema(t, prompt.BuildLabels(prompt.LabelsInput{}))
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{"a promise labelling two functions", `{"labels":{"p1":["f1","f2"]}}`, true},
		{"a promise labelling nothing", `{"labels":{"p1":[]}}`, true},
		{"no labels at all", `{"labels":{}}`, true},
		{"a promise id that is not an id", `{"labels":{"promise":["f1"]}}`, false},
		{"a function id that is not an id", `{"labels":{"p1":["parse"]}}`, false},
		{"a function listed twice", `{"labels":{"p1":["f1","f1"]}}`, false},
		{"a promise mapped to a string", `{"labels":{"p1":"f1"}}`, false},
		{"an extra top-level member", `{"labels":{},"notes":"n"}`, false},
		{"no labels member", `{}`, false},
	}
	for _, c := range cases {
		t.Run(c.name+verifiesSP2164, func(t *testing.T) {
			if got := schemaAccepts(t, schema, c.output); got != c.want {
				t.Errorf("schema accepts = %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseLabels(t *testing.T) {
	promises := []string{"p1", "p2"}
	functions := []string{"f1", "f2", "f3"}
	t.Run("each promise gets the functions it labels"+verifiesSP2165, func(t *testing.T) {
		got, err := prompt.ParseLabels([]byte(`{"labels":{"p1":["f3","f1"],"p2":["f2"]}}`), promises, functions)
		if err != nil || !slices.Equal(got["p1"], []string{"f1", "f3"}) || !slices.Equal(got["p2"], []string{"f2"}) {
			t.Errorf("got %+v, %v", got, err)
		}
	})
	t.Run("a promise the response leaves out labels no function"+verifiesSP2165, func(t *testing.T) {
		got, err := prompt.ParseLabels([]byte(`{"labels":{"p1":["f1"]}}`), promises, functions)
		if err != nil || len(got) != 2 || len(got["p2"]) != 0 {
			t.Errorf("got %+v, %v", got, err)
		}
	})
	t.Run("a function id outside the view is refused"+verifiesSP2165, func(t *testing.T) {
		_, err := prompt.ParseLabels([]byte(`{"labels":{"p1":["f1","f9"]}}`), promises, functions)
		if !errors.Is(err, prompt.ErrUnknownLabelFunction) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("a promise id outside the input is refused"+verifiesSP2165, func(t *testing.T) {
		_, err := prompt.ParseLabels([]byte(`{"labels":{"p9":["f1"]}}`), promises, functions)
		if !errors.Is(err, prompt.ErrUnknownLabelPromise) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("a response that is not JSON is refused"+verifiesSP2165, func(t *testing.T) {
		if _, err := prompt.ParseLabels([]byte(brokenJSON), promises, functions); err == nil {
			t.Error("accepted")
		}
	})
	t.Run("the labels hold each function once in view order whatever the response order"+verifiesSP2165, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			chosen := rapid.SliceOfNDistinct(rapid.SampledFrom(functions), 0, len(functions), rapid.ID[string]).Draw(t, "chosen")
			repeated := append(slices.Clone(chosen), chosen...)
			encoded, err := json.Marshal(map[string]map[string][]string{"labels": {"p1": repeated}})
			if err != nil {
				t.Fatal(err)
			}
			got, err := prompt.ParseLabels(encoded, promises, functions)
			if err != nil {
				t.Fatal(err)
			}
			want := slices.DeleteFunc(slices.Clone(functions), func(id string) bool { return !slices.Contains(chosen, id) })
			if !slices.Equal(got["p1"], want) {
				t.Fatalf("got %v, want %v", got["p1"], want)
			}
		})
	})
}
