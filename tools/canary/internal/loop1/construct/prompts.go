package construct

import (
	"bytes"
	"embed"
	"fmt"
	"path"
	"text/template"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

//go:embed prompts/*.tmpl prompts/files.schema.json
var promptFiles embed.FS

const (
	promptDir      = "prompts"
	schemaFile     = "files.schema.json"
	authorTemplate = "author.tmpl"
	variantsName   = "variants.tmpl"
	testsTemplate  = "tests.tmpl"
)

// FilesSchema is the schema of every call's final message.
func FilesSchema() (Schema, error) {
	data, err := promptFiles.ReadFile(path.Join(promptDir, schemaFile))
	if err != nil {
		return Schema{}, fmt.Errorf("read the output schema: %w", err)
	}
	return Schema{Name: schemaFile, JSON: data}, nil
}

// Prompts renders the prompts of the roles.
type Prompts struct {
	templates *template.Template
}

// NewPrompts parses the embedded templates.
func NewPrompts() (Prompts, error) {
	parsed, err := template.ParseFS(promptFiles, path.Join(promptDir, "*.tmpl"))
	if err != nil {
		return Prompts{}, fmt.Errorf("parse the prompt templates: %w", err)
	}
	return Prompts{templates: parsed}, nil
}

func (p Prompts) render(name string, data any) (string, error) {
	var out bytes.Buffer
	if err := p.templates.ExecuteTemplate(&out, name, data); err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}
	return out.String(), nil
}

// AuthorInput is what the author is told about a domain: the domain line, the
// shape rules (the template itself) and a project of the same shape, and, for a
// repair, its previous files with the problems found in them.
type AuthorInput struct {
	Domain        Domain
	ExamplePrompt string
	Example       []File
	Previous      []File
	Problems      []string
}

type authorData struct {
	AuthorInput
	PromptTemplate string
}

// Author renders the prompt that asks for a whole task.
func (p Prompts) Author(input AuthorInput) (string, error) {
	return p.render(authorTemplate, authorData{AuthorInput: input, PromptTemplate: accept.PromptTemplate})
}

// VariantsInput is what the author is told when only the violations and the
// refactors are written: the spec and the source of the implementation.
type VariantsInput struct {
	Spec string
	// Source is the module's path in the project; ModuleFile its path relative to
	// the package directory, where the variants lie.
	Source     string
	ModuleFile string
	Reference  []File
	Previous   []File
	Problems   []string
}

// Variants renders the prompt that asks for violations and refactors alone.
func (p Prompts) Variants(input VariantsInput) (string, error) {
	return p.render(variantsName, input)
}

// TestsInput is everything the test writer is told: the spec, the interface of
// the code under test, and, for a repair, its own previous files with the
// problems found in them. It holds no member that could carry the project's
// source, tests, reference, violations or refactors.
type TestsInput struct {
	Spec      string
	Interface study.Interface
	Previous  []File
	Problems  []string
}

type testsData struct {
	TestsInput
	Files []testFile
}

type testFile struct{ Criterion, Name string }

// Tests renders the prompt that asks for the tests of a spec.
func (p Prompts) Tests(input TestsInput) (string, error) {
	var files []testFile
	for _, criterion := range accept.ParseCriteria([]byte(input.Spec)) {
		files = append(files, testFile{Criterion: criterion.ID, Name: accept.OracleFileName(criterion.ID)})
	}
	if len(files) == 0 {
		return "", fmt.Errorf("the spec declares no criterion")
	}
	return p.render(testsTemplate, testsData{TestsInput: input, Files: files})
}
