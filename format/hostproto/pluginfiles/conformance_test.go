package pluginfiles_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/dlclark/regexp2"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto/pluginfiles"
)

const (
	repositoryRoot = "../../.."
	schemaDir      = "testdata/agent-plugins-1.0.0"
	sourceFile     = "SOURCE.json"

	skillFile = "SKILL.md"
	// Limits of the Agent Skills format.
	maxSkillNameLength        = 64
	maxSkillDescriptionLength = 1024
	maxSkillCompatibility     = 500
	skillFence                = "---\n"
)

// portableTrees are the directories of the repository that hold a plugin of the
// Agent Plugins standard, relative to the repository root.
var portableTrees = []string{
	"alignment/plugin/portable",
	"completeness/plugin/portable",
	"discpower/plugin/portable",
	"s1relay/plugin/portable",
	"umbrella/plugin/portable",
}

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// skillFields are the front matter fields the Agent Skills format defines.
var skillFields = []string{"name", "description", "license", "compatibility", "metadata", "allowed-tools"}

type vendoredSource struct {
	Schemas []struct {
		File   string `json:"file"`
		ID     string `json:"id"`
		SHA256 string `json:"sha256"`
		URL    string `json:"url"`
	} `json:"schemas"`
	Specification struct {
		Commit     string `json:"commit"`
		Repository string `json:"repository"`
	} `json:"specification"`
}

func readSource(t *testing.T) vendoredSource {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(schemaDir, sourceFile))
	if err != nil {
		t.Fatal(err)
	}
	var source vendoredSource
	if err := json.Unmarshal(data, &source); err != nil {
		t.Fatal(err)
	}
	return source
}

// ecmaRegexp lets the schema's name pattern, which uses a lookahead the standard
// library lacks, compile.
type ecmaRegexp regexp2.Regexp

func (re *ecmaRegexp) MatchString(s string) bool {
	matched, err := (*regexp2.Regexp)(re).MatchString(s)
	return err == nil && matched
}

func (re *ecmaRegexp) String() string { return (*regexp2.Regexp)(re).String() }

func compileECMA(pattern string) (jsonschema.Regexp, error) {
	re, err := regexp2.Compile(pattern, regexp2.ECMAScript)
	if err != nil {
		return nil, err
	}
	return (*ecmaRegexp)(re), nil
}

// schemaOf compiles a vendored schema from its file, with no network access.
func schemaOf(t *testing.T, file string) *jsonschema.Schema {
	t.Helper()
	handle, err := os.Open(filepath.Join(schemaDir, file))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = handle.Close() }()
	document, err := jsonschema.UnmarshalJSON(handle)
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseRegexpEngine(compileECMA)
	if err := compiler.AddResource(file, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(file)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func validate(t *testing.T, schema *jsonschema.Schema, data []byte) error {
	t.Helper()
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	return schema.Validate(instance)
}

func TestVendoredSchemas(t *testing.T) {
	t.Run("the vendored schemas are the files of the pinned release, by digest, and each declares its canonical id [verifies SP-1703~1]", func(t *testing.T) {
		source := readSource(t)
		if len(source.Specification.Commit) != 40 || !strings.HasPrefix(source.Specification.Repository, "https://") || len(source.Schemas) != 2 {
			t.Fatalf("source = %+v", source)
		}
		for _, entry := range source.Schemas {
			data, err := os.ReadFile(filepath.Join(schemaDir, entry.File))
			if err != nil {
				t.Fatal(err)
			}
			if digest := format.SHA256Hex(data); digest != entry.SHA256 {
				t.Errorf("%s: digest %s, recorded %s", entry.File, digest, entry.SHA256)
			}
			if !strings.Contains(entry.URL, source.Specification.Commit) || !strings.HasSuffix(entry.URL, "/schemas/1.0.0/"+entry.File) {
				t.Errorf("%s: source url %s", entry.File, entry.URL)
			}
			var document struct {
				ID string `json:"$id"`
			}
			if err := json.Unmarshal(data, &document); err != nil || document.ID != entry.ID {
				t.Errorf("%s: $id %q, recorded %q (%v)", entry.File, document.ID, entry.ID, err)
			}
		}
		if source.Schemas[0].ID != pluginfiles.PluginSchemaURL || source.Schemas[1].ID != pluginfiles.MCPSchemaURL {
			t.Errorf("the generator names %s and %s", pluginfiles.PluginSchemaURL, pluginfiles.MCPSchemaURL)
		}
	})

	t.Run("the manifest schema refuses what the standard refuses and takes what it takes [verifies SP-1703~1]", func(t *testing.T) {
		schema := schemaOf(t, "plugin.schema.json")
		const head = `{"$schema":"` + pluginfiles.PluginSchemaURL + `",`
		accepted := []string{
			head + `"name":"a"}`,
			head + `"name":"acme.tools","version":"1","author":{"name":"n"},"extensions":{"com.example.client":{"x":true}}}`,
		}
		refused := []string{
			head + `"name":"My-Plugin"}`, head + `"name":"has--double"}`, head + `"name":"-start"}`, head + `"name":""}`,
			head + `"name":"a","unknown":1}`, head + `"name":"a","author":{"nick":"n"}}`, head + `"name":"a","extensions":{"x":1}}`,
			`{"$schema":"https://agent-plugins.org/schemas/9.9.9/plugin.schema.json","name":"a"}`, `{"name":"a"}`,
		}
		for _, document := range accepted {
			if err := validate(t, schema, []byte(document)); err != nil {
				t.Errorf("%s was refused: %v", document, err)
			}
		}
		for _, document := range refused {
			if err := validate(t, schema, []byte(document)); err == nil {
				t.Errorf("%s was accepted", document)
			}
		}
	})
}

// portableTreesOnDisk lists the directories under the repository root that hold a
// root `plugin.json` of the standard: the manifests of other tools sit in dot
// directories, and fixtures and the experiments are not packages.
func portableTreesOnDisk(t *testing.T) []string {
	t.Helper()
	var trees []string
	err := filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		if entry.IsDir() {
			skipped := path != repositoryRoot && strings.HasPrefix(name, ".") ||
				name == "experiments" || name == "testdata" || name == "node_modules" || name == "build"
			if skipped {
				return fs.SkipDir
			}
			return nil
		}
		if name == pluginfiles.PortableManifestPath {
			relative, err := filepath.Rel(repositoryRoot, filepath.Dir(path))
			trees = append(trees, filepath.ToSlash(relative))
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(trees)
	return trees
}

func TestCommittedPluginsConform(t *testing.T) {
	t.Run("every committed plugin of the standard is one the repository lists [verifies SP-1703~1]", func(t *testing.T) {
		if got := portableTreesOnDisk(t); !slices.Equal(got, portableTrees) {
			t.Fatalf("plugin trees on disk = %v, want %v", got, portableTrees)
		}
	})

	pluginSchema, mcpSchema := schemaOf(t, "plugin.schema.json"), schemaOf(t, "mcp.schema.json")
	for _, tree := range portableTrees {
		root := filepath.Join(repositoryRoot, filepath.FromSlash(tree))

		t.Run(tree+" has a root manifest valid against the official schema [verifies SP-1703~1]", func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, pluginfiles.PortableManifestPath))
			if err != nil {
				t.Fatal(err)
			}
			if err := validate(t, pluginSchema, data); err != nil {
				t.Fatalf("plugin.json: %v", err)
			}
		})

		t.Run(tree+" has an mcp.json, when it has one, valid against the official schema and of the manifest's version [verifies SP-1703~1]", func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, pluginfiles.PortableMCPPath))
			if os.IsNotExist(err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := validate(t, mcpSchema, data); err != nil {
				t.Fatalf("mcp.json: %v", err)
			}
			var document struct {
				Servers map[string]struct {
					Command string `json:"command"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			for name, server := range document.Servers {
				if strings.Contains(server.Command, " ") || strings.HasPrefix(server.Command, "../") || filepath.IsAbs(server.Command) {
					t.Errorf("server %s: command %q is neither a bare name nor a plugin-relative path", name, server.Command)
				}
			}
		})

		t.Run(tree+" has skills in the Agent Skills format [verifies SP-1703~1]", func(t *testing.T) {
			skills, err := os.ReadDir(filepath.Join(root, pluginfiles.PortableSkillsDirectory))
			if err != nil || len(skills) == 0 {
				t.Fatalf("skills: %v, %d found", err, len(skills))
			}
			for _, skill := range skills {
				checkSkill(t, filepath.Join(root, pluginfiles.PortableSkillsDirectory, skill.Name()), skill.Name())
			}
		})
	}

	t.Run("a skill that breaks the Agent Skills format is found [verifies SP-1703~1]", func(t *testing.T) {
		valid := "name: a-skill\ndescription: Does a thing. Use when asked.\n"
		for name, front := range map[string]string{
			"a name that is not its directory": strings.Replace(valid, "a-skill", "other", 1),
			"an uppercase name":                strings.Replace(valid, "a-skill", "A-skill", 1),
			"a doubled hyphen":                 strings.Replace(valid, "a-skill", "a--skill", 1),
			"no description":                   "name: a-skill\n",
			"an unknown field":                 valid + "triggers: x\n",
			"a description over the limit":     "name: a-skill\ndescription: " + strings.Repeat("x", maxSkillDescriptionLength+1) + "\n",
		} {
			if problems := skillProblems(skillFence+front+skillFence+"\nbody\n", "a-skill"); len(problems) == 0 {
				t.Errorf("%s was accepted", name)
			}
		}
		if problems := skillProblems(skillFence+valid+skillFence+"\nbody\n", "a-skill"); len(problems) != 0 {
			t.Errorf("a valid skill was refused: %v", problems)
		}
	})
}

func checkSkill(t *testing.T, directory, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, skillFile))
	if err != nil {
		t.Errorf("skill %s: %v", name, err)
		return
	}
	for _, problem := range skillProblems(string(data), name) {
		t.Errorf("skill %s: %s", name, problem)
	}
}

// skillProblems lists what is wrong with a SKILL.md against the Agent Skills
// format, given the name of the directory that holds it.
func skillProblems(text, directory string) []string {
	rest, found := strings.CutPrefix(text, skillFence)
	front, _, closed := strings.Cut(rest, "\n"+skillFence)
	if !found || !closed {
		return []string{"no YAML front matter"}
	}
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(front), &fields); err != nil {
		return []string{"front matter is not YAML: " + err.Error()}
	}
	var problems []string
	for field := range fields {
		if !slices.Contains(skillFields, field) {
			problems = append(problems, "unknown field "+field)
		}
	}
	name, _ := fields["name"].(string)
	switch {
	case name == "" || len(name) > maxSkillNameLength || !skillNamePattern.MatchString(name):
		problems = append(problems, "name "+name+" is not 1-64 lowercase letters, digits and single hyphens")
	case name != directory:
		problems = append(problems, "name "+name+" is not the directory "+directory)
	}
	description, _ := fields["description"].(string)
	if description == "" || len(description) > maxSkillDescriptionLength {
		problems = append(problems, "description is not 1-1024 characters")
	}
	if compatibility, present := fields["compatibility"]; present {
		if text, _ := compatibility.(string); text == "" || len(text) > maxSkillCompatibility {
			problems = append(problems, "compatibility is not 1-500 characters")
		}
	}
	if allowed, present := fields["allowed-tools"]; present {
		if _, isText := allowed.(string); !isText {
			problems = append(problems, "allowed-tools is not a space-separated string")
		}
	}
	return problems
}
