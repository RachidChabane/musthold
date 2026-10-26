package domain_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/RachidChabane/musthold/tools/release/internal/domain"
)

const (
	verifiesSBOM    = " [verifies SP-1807~1]"
	verifiesFormula = " [verifies SP-1808~1]"
	verifiesNotes   = " [verifies SP-1809~1]"
	goldenDirectory = "testdata"
	spdxSchemaPath  = "../../testdata/spdx-2.3/spdx-schema.json"
	spdxSchemaID    = "http://spdx.org/rdf/terms/2.3"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join(goldenDirectory, name)
	if *update {
		if err := os.MkdirAll(goldenDirectory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden file; run the test with -update to review the change\n--- got\n%s", name, got)
	}
}

var commitTime = time.Unix(1_700_000_000, 0).UTC()

func sbomInput(t *testing.T) domain.SBOMInput {
	t.Helper()
	tag := mustTag(t, "discpower/v0.2.0")
	return domain.SBOMInput{
		Tag:         tag,
		ArchiveName: "musthold-dp_0.2.0_linux_amd64.tar.gz",
		Created:     commitTime,
		Build: domain.BuildFacts{
			GoVersion: "go1.26.4", MainPath: "github.com/RachidChabane/musthold/discpower", MainVersion: "(devel)",
			Dependencies: []domain.Dependency{
				{Path: "pgregory.net/rapid", Version: "v1.3.0", Sum: "h1:rapid"},
				{Path: "github.com/RachidChabane/musthold/format", Version: "v0.1.0", Sum: "h1:format"},
				{Path: "gopkg.in/yaml.v3", Version: "v3.0.1", Sum: "h1:yaml"},
			},
			Settings: []domain.BuildSetting{{Key: "GOOS", Value: "linux"}, {Key: "-trimpath", Value: "true"}, {Key: "CGO_ENABLED", Value: "0"}},
		},
	}
}

func TestSBOM(t *testing.T) {
	render := func(t *testing.T, input domain.SBOMInput) []byte {
		t.Helper()
		data, err := domain.RenderSPDX(input)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	t.Run("the SBOM matches its golden rendering"+verifiesSBOM, func(t *testing.T) {
		assertGolden(t, "sbom.golden.json", render(t, sbomInput(t)))
	})

	t.Run("the SBOM validates against the SPDX 2.3 schema"+verifiesSBOM, func(t *testing.T) {
		schemaData, err := os.ReadFile(spdxSchemaPath)
		if err != nil {
			t.Fatal(err)
		}
		schemaDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
		if err != nil {
			t.Fatal(err)
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(spdxSchemaID, schemaDocument); err != nil {
			t.Fatal(err)
		}
		schema, err := compiler.Compile(spdxSchemaID)
		if err != nil {
			t.Fatal(err)
		}
		instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(render(t, sbomInput(t))))
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(instance); err != nil {
			t.Fatalf("the SBOM is not valid SPDX 2.3: %v", err)
		}
	})

	t.Run("the schema rejects an SBOM that lacks its document namespace"+verifiesSBOM, func(t *testing.T) {
		schemaData, err := os.ReadFile(spdxSchemaPath)
		if err != nil {
			t.Fatal(err)
		}
		schemaDocument, _ := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
		compiler := jsonschema.NewCompiler()
		_ = compiler.AddResource(spdxSchemaID, schemaDocument)
		schema, err := compiler.Compile(spdxSchemaID)
		if err != nil {
			t.Fatal(err)
		}
		instance, _ := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(`{"spdxVersion":"SPDX-2.3"}`)))
		if schema.Validate(instance) == nil {
			t.Fatal("an incomplete document validated: the schema check proves nothing")
		}
	})

	t.Run("the SBOM does not depend on the order of the dependencies or the settings"+verifiesSBOM, func(t *testing.T) {
		input := sbomInput(t)
		shuffled := input
		shuffled.Build.Dependencies = []domain.Dependency{input.Build.Dependencies[2], input.Build.Dependencies[0], input.Build.Dependencies[1]}
		shuffled.Build.Settings = []domain.BuildSetting{input.Build.Settings[2], input.Build.Settings[0], input.Build.Settings[1]}
		if !bytes.Equal(render(t, input), render(t, shuffled)) {
			t.Fatal("the order of the inputs changed the SBOM")
		}
		if !bytes.Equal(render(t, input), render(t, input)) {
			t.Fatal("rendering twice differs")
		}
	})

	t.Run("the SBOM carries the commit time and names its archive in its namespace"+verifiesSBOM, func(t *testing.T) {
		header, err := domain.ParseSPDXHeader(render(t, sbomInput(t)))
		if err != nil {
			t.Fatal(err)
		}
		wantNamespace := "https://github.com/RachidChabane/musthold/discpower/v0.2.0/musthold-dp_0.2.0_linux_amd64.tar.gz"
		if header.Version != "SPDX-2.3" || header.Namespace != wantNamespace || header.Created != "2023-11-14T22:13:20Z" {
			t.Fatalf("header = %+v", header)
		}
	})

	t.Run("a document that is not JSON has no header"+verifiesSBOM, func(t *testing.T) {
		if _, err := domain.ParseSPDXHeader([]byte("not json")); err == nil {
			t.Fatal("parsed")
		}
	})
}

func TestFormula(t *testing.T) {
	tag := mustTag(t, "discpower/v0.2.0")
	var sums []domain.Checksum
	for _, target := range domain.Targets() {
		archive := target.ArchiveName("musthold-dp", tag.Version)
		sums = append(sums, domain.Checksum{Digest: domain.DigestOf([]byte(archive)), Name: archive})
	}

	t.Run("the formula matches its golden rendering"+verifiesFormula, func(t *testing.T) {
		formula, err := domain.RenderFormula(tag, sums)
		if err != nil {
			t.Fatal(err)
		}
		assertGolden(t, "formula.golden.rb", []byte(formula))
	})

	t.Run("a checksum list that lacks an archive is refused"+verifiesFormula, func(t *testing.T) {
		if _, err := domain.RenderFormula(tag, sums[1:]); err == nil {
			t.Fatal("rendered")
		}
	})

	t.Run("a module that ships no binary has no formula"+verifiesFormula, func(t *testing.T) {
		for _, text := range []string{"format/v1.0.0", "umbrella/v1.0.0"} {
			if _, err := domain.RenderFormula(mustTag(t, text), sums); err == nil {
				t.Errorf("%s rendered", text)
			}
		}
	})

	t.Run("the download url keeps the slash of the tag"+verifiesFormula, func(t *testing.T) {
		got := domain.DownloadURL(tag, "x.tar.gz")
		if got != "https://github.com/RachidChabane/musthold/releases/download/discpower/v0.2.0/x.tar.gz" {
			t.Fatalf("url = %q", got)
		}
	})
}

func TestNotes(t *testing.T) {
	commits := []domain.Commit{{Hash: "abc1234", Subject: "feat(dp): add a thing"}, {Hash: "def5678", Subject: "fix(format): mend it"}}

	t.Run("the notes of a binary release match their golden rendering"+verifiesNotes, func(t *testing.T) {
		notes := domain.RenderNotes(domain.NotesInput{Tag: mustTag(t, "discpower/v0.2.0"), Previous: "discpower/v0.1.0", Commits: commits})
		assertGolden(t, "notes-binary.golden.md", []byte(notes))
	})

	t.Run("the notes of the library and of the plugin say how to get what they ship"+verifiesNotes, func(t *testing.T) {
		library := domain.RenderNotes(domain.NotesInput{Tag: mustTag(t, "format/v1.0.0")})
		assertGolden(t, "notes-library.golden.md", []byte(library))
		plugin := domain.RenderNotes(domain.NotesInput{Tag: mustTag(t, "umbrella/v1.0.0"), Commits: commits[:1]})
		assertGolden(t, "notes-plugin.golden.md", []byte(plugin))
	})
}
