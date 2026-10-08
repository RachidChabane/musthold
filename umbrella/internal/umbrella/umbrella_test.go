package umbrella_test

import (
	"encoding/json"
	"flag"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/umbrella/internal/umbrella"
)

// update rewrites the committed files from the generator.
var update = flag.Bool("update", false, "rewrite the committed umbrella files from the generator")

const (
	umbrellaRoot   = "../.."
	repositoryRoot = "../../.."
	frontFence     = "---\n"
	hooksFile      = "hooks/hooks.json"
	manifestFile   = ".claude-plugin/plugin.json"
)

// moduleVersions reads the version each module's committed Claude Code plugin
// carries, the single source of the release tag the marketplace pins.
func moduleVersions(t *testing.T) umbrella.Versions {
	t.Helper()
	versions := umbrella.Versions{}
	for _, module := range append(append([]umbrella.Module{}, umbrella.Modules...), umbrella.Relay) {
		path := filepath.Join(repositoryRoot, module.Directory, "plugin", "claude", filepath.FromSlash(manifestFile))
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var manifest struct{ Version string }
		if err := json.Unmarshal(content, &manifest); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		versions[module.Directory] = manifest.Version
	}
	return versions
}

func generated(t *testing.T) map[string]string {
	t.Helper()
	files, err := umbrella.Files(moduleVersions(t))
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]string{}
	for _, file := range files {
		if _, duplicate := byPath[file.Path]; duplicate {
			t.Fatalf("%s is generated twice", file.Path)
		}
		byPath[file.Path] = string(file.Content)
	}
	return byPath
}

// committedFiles reads every file below the paths of the umbrella that the
// generator owns: the marketplace directory and the tree of the thin plugin.
func committedFiles(t *testing.T) map[string]string {
	t.Helper()
	byPath := map[string]string{}
	for _, owned := range []string{".claude-plugin", "plugins"} {
		root := filepath.Join(umbrellaRoot, owned)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return walkErr
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(umbrellaRoot, path)
			if err != nil {
				return err
			}
			byPath[filepath.ToSlash(relative)] = string(content)
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	return byPath
}

func generatedRoot(t *testing.T) map[string]string {
	t.Helper()
	files, err := umbrella.RootFiles(moduleVersions(t))
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]string{}
	for _, file := range files {
		byPath[file.Path] = string(file.Content)
	}
	return byPath
}

// committedRootFiles reads every file below the paths of the repository root
// that the generator owns: the marketplace directory the root is added by.
func committedRootFiles(t *testing.T) map[string]string {
	t.Helper()
	byPath := map[string]string{}
	root := filepath.Join(repositoryRoot, ".claude-plugin")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		byPath[filepath.ToSlash(relative)] = string(content)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return byPath
}

func TestGeneratedRootFiles(t *testing.T) {
	t.Run("the marketplace at the repository root is what the generator produces, byte for byte, and generating twice gives the same bytes [verifies SP-906~1]", func(t *testing.T) {
		want := generatedRoot(t)
		if *update {
			if err := os.RemoveAll(filepath.Join(repositoryRoot, ".claude-plugin")); err != nil {
				t.Fatal(err)
			}
			for path, content := range want {
				target := filepath.Join(repositoryRoot, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		got := committedRootFiles(t)
		for path, content := range want {
			if got[path] != content {
				t.Errorf("%s differs from the generator: run script/plugin-gen", path)
			}
		}
		for path := range got {
			if _, expected := want[path]; !expected {
				t.Errorf("%s is committed but not generated", path)
			}
		}
		if !slices.Equal(slices.Sorted(maps.Keys(want)), []string{umbrella.RootMarketplacePath}) {
			t.Errorf("the root files are %v", slices.Sorted(maps.Keys(want)))
		}
		again := generatedRoot(t)
		for path, content := range want {
			if again[path] != content {
				t.Errorf("%s differs between two runs", path)
			}
		}
	})
}

func readRootListing(t *testing.T) listing {
	t.Helper()
	var found listing
	if err := json.Unmarshal([]byte(generatedRoot(t)[umbrella.RootMarketplacePath]), &found); err != nil {
		t.Fatal(err)
	}
	return found
}

func TestRootMarketplace(t *testing.T) {
	root, own := readRootListing(t), readListing(t)

	t.Run("the root marketplace lists the plugins of the umbrella's own, in the same order, and differs from it in the path of the thin plugin alone [verifies SP-906~1]", func(t *testing.T) {
		if root.Name != own.Name || root.Owner != own.Owner || len(root.Plugins) != len(own.Plugins) {
			t.Fatalf("root = %+v, own = %+v", root, own)
		}
		last := len(root.Plugins) - 1
		for index := range root.Plugins[:last] {
			if root.Plugins[index].Name != own.Plugins[index].Name || string(root.Plugins[index].Source) != string(own.Plugins[index].Source) ||
				root.Plugins[index].Description != own.Plugins[index].Description {
				t.Errorf("plugin %d is %+v at the root and %+v in the umbrella", index, root.Plugins[index], own.Plugins[index])
			}
		}
		if root.Plugins[last].Name != own.Plugins[last].Name || root.Plugins[last].Description != own.Plugins[last].Description {
			t.Errorf("the thin plugin is %+v at the root and %+v in the umbrella", root.Plugins[last], own.Plugins[last])
		}
	})

	t.Run("the thin plugin is reached from the repository root, and the tree it names holds its manifest [verifies SP-906~1]", func(t *testing.T) {
		thin := root.Plugins[len(root.Plugins)-1]
		path, relative := thin.relative()
		if !relative || path != "./"+umbrella.UmbrellaDirectory+"/"+umbrella.PluginDirectory {
			t.Fatalf("source = %s", thin.Source)
		}
		manifest := filepath.Join(repositoryRoot, filepath.FromSlash(path), filepath.FromSlash(manifestFile))
		if _, err := os.Stat(manifest); err != nil {
			t.Fatalf("the root marketplace points at a tree without a manifest: %v", err)
		}
	})

	t.Run("every plugin the root marketplace lists by a path in this repository has its committed tree [verifies SP-906~1]", func(t *testing.T) {
		for _, plugin := range root.Plugins {
			if path, relative := plugin.relative(); relative {
				if _, err := os.Stat(filepath.Join(repositoryRoot, filepath.FromSlash(path), filepath.FromSlash(manifestFile))); err != nil {
					t.Errorf("%s: %v", plugin.Name, err)
				}
				continue
			}
			if source := plugin.subdir(t); source.URL == "RachidChabane/spec-proof" {
				if _, err := os.Stat(filepath.Join(repositoryRoot, filepath.FromSlash(source.Path), filepath.FromSlash(manifestFile))); err != nil {
					t.Errorf("%s: %v", plugin.Name, err)
				}
			}
		}
	})
}

func TestGeneratedFiles(t *testing.T) {
	t.Run("the committed files are what the generator produces, byte for byte, and generating twice gives the same bytes [verifies SP-820~1]", func(t *testing.T) {
		want := generated(t)
		if *update {
			for _, owned := range []string{".claude-plugin", "plugins"} {
				if err := os.RemoveAll(filepath.Join(umbrellaRoot, owned)); err != nil {
					t.Fatal(err)
				}
			}
			for path, content := range want {
				target := filepath.Join(umbrellaRoot, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		got := committedFiles(t)
		for path, content := range want {
			if got[path] != content {
				t.Errorf("%s differs from the generator: run script/plugin-gen", path)
			}
		}
		for path := range got {
			if _, expected := want[path]; !expected {
				t.Errorf("%s is committed but not generated", path)
			}
		}
		again := generated(t)
		for path, content := range want {
			if again[path] != content {
				t.Errorf("%s differs between two runs", path)
			}
		}
	})
}

type listedPlugin struct {
	Name        string          `json:"name"`
	Source      json.RawMessage `json:"source"`
	Description string          `json:"description"`
}

type listing struct {
	Name  string `json:"name"`
	Owner struct {
		Name string `json:"name"`
	} `json:"owner"`
	Plugins []listedPlugin `json:"plugins"`
}

func readListing(t *testing.T) listing {
	t.Helper()
	var found listing
	decoder := json.NewDecoder(strings.NewReader(generated(t)[umbrella.MarketplacePath]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&found); err == nil {
		return found
	}
	// metadata is allowed: decode again without the strictness
	if err := json.Unmarshal([]byte(generated(t)[umbrella.MarketplacePath]), &found); err != nil {
		t.Fatal(err)
	}
	return found
}

type subdirSource struct {
	Source string `json:"source"`
	URL    string `json:"url"`
	Path   string `json:"path"`
	Ref    string `json:"ref"`
}

func (p listedPlugin) subdir(t *testing.T) subdirSource {
	t.Helper()
	var source subdirSource
	if err := json.Unmarshal(p.Source, &source); err != nil {
		t.Fatalf("%s: the source is not an object: %v", p.Name, err)
	}
	return source
}

func (p listedPlugin) relative() (string, bool) {
	var path string
	if err := json.Unmarshal(p.Source, &path); err != nil {
		return "", false
	}
	return path, true
}

func TestMarketplace(t *testing.T) {
	found := readListing(t)

	t.Run("the marketplace is named spec-proof, has an owner and lists each plugin once, Shallnot's first and the thin plugin last [verifies SP-821~2]", func(t *testing.T) {
		if found.Name != umbrella.MarketplaceName || found.Owner.Name == "" {
			t.Fatalf("marketplace = %+v", found)
		}
		var names []string
		seen := map[string]bool{}
		for _, plugin := range found.Plugins {
			if seen[plugin.Name] {
				t.Errorf("%s is listed twice", plugin.Name)
			}
			seen[plugin.Name] = true
			names = append(names, plugin.Name)
			if plugin.Description == "" {
				t.Errorf("%s has no description", plugin.Name)
			}
		}
		want := []string{"shallnot", "spec-proof-dp", "spec-proof-alignment", "spec-proof-completeness", "spec-proof-s1", "spec-proof"}
		if !slices.Equal(names, want) {
			t.Fatalf("plugins = %v, want %v", names, want)
		}
	})

	t.Run("Shallnot's plugin is referenced by its public marketplace source and the modules by their committed trees at the release tag of the version their plugin carries [verifies SP-821~2]", func(t *testing.T) {
		shallnot := found.Plugins[0].subdir(t)
		if shallnot != (subdirSource{Source: "git-subdir", URL: "RachidChabane/shallnot", Path: "plugin"}) {
			t.Fatalf("shallnot source = %+v", shallnot)
		}
		versions := moduleVersions(t)
		modules := append(append([]umbrella.Module{}, umbrella.Modules...), umbrella.Relay)
		for index, module := range modules {
			source := found.Plugins[index+1].subdir(t)
			want := subdirSource{
				Source: "git-subdir", URL: "RachidChabane/spec-proof", Path: module.Directory + "/plugin/claude",
				Ref: module.Directory + "/v" + versions[module.Directory],
			}
			if source != want {
				t.Errorf("%s source = %+v, want %+v", module.Plugin, source, want)
			}
		}
	})

	t.Run("a module without a plugin version makes the generator fail instead of writing an unpinned entry [verifies SP-821~2]", func(t *testing.T) {
		if _, err := umbrella.Files(umbrella.Versions{}); err == nil {
			t.Fatal("the marketplace was generated with no module version")
		}
		if _, err := umbrella.RootFiles(umbrella.Versions{"discpower": ""}); err == nil {
			t.Fatal("the root marketplace was generated with an empty module version")
		}
	})

	t.Run("a release tag reads module/vVERSION [verifies SP-821~2]", func(t *testing.T) {
		if got := umbrella.ReleaseTag("discpower", "0.1.0"); got != "discpower/v0.1.0" {
			t.Fatalf("tag = %q", got)
		}
	})

	t.Run("the thin plugin is a relative path to its own tree, which exists in the umbrella [verifies SP-821~2]", func(t *testing.T) {
		thin := found.Plugins[len(found.Plugins)-1]
		path, relative := thin.relative()
		if !relative || path != "./"+umbrella.PluginDirectory {
			t.Fatalf("source = %s", thin.Source)
		}
		if !strings.HasPrefix(path, "./") || strings.Contains(path, "..") {
			t.Fatalf("%s leaves the marketplace", path)
		}
	})
}

type thinManifest struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	Dependencies []string `json:"dependencies"`
}

func TestThinPlugin(t *testing.T) {
	files := generated(t)
	var manifest thinManifest
	if err := json.Unmarshal([]byte(files[umbrella.PluginDirectory+"/"+manifestFile]), &manifest); err != nil {
		t.Fatal(err)
	}

	t.Run("the thin plugin is named spec-proof and depends on Shallnot, the three modules and the relay, each an entry of the marketplace [verifies SP-822~1]", func(t *testing.T) {
		if manifest.Name != umbrella.PluginName || !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(manifest.Version) || manifest.Description == "" {
			t.Fatalf("manifest = %+v", manifest)
		}
		want := []string{"shallnot", "spec-proof-dp", "spec-proof-alignment", "spec-proof-completeness", "spec-proof-s1"}
		if !slices.Equal(manifest.Dependencies, want) {
			t.Fatalf("dependencies = %v, want %v", manifest.Dependencies, want)
		}
		listed := map[string]bool{}
		for _, plugin := range readListing(t).Plugins {
			listed[plugin.Name] = true
		}
		for _, dependency := range manifest.Dependencies {
			if !listed[dependency] || dependency == manifest.Name {
				t.Errorf("dependency %s is not an entry of the marketplace other than the plugin itself", dependency)
			}
		}
	})

	t.Run("the thin plugin holds one skill and no hook, agent, command or server [verifies SP-822~1]", func(t *testing.T) {
		var paths []string
		for path := range files {
			if strings.HasPrefix(path, umbrella.PluginDirectory+"/") {
				paths = append(paths, strings.TrimPrefix(path, umbrella.PluginDirectory+"/"))
			}
		}
		slices.Sort(paths)
		want := []string{manifestFile, "skills/" + umbrella.SkillName + "/SKILL.md"}
		if !slices.Equal(paths, want) {
			t.Fatalf("files = %v, want %v", paths, want)
		}
		for _, forbidden := range []string{"hooks", "agents", "commands", ".mcp.json"} {
			if _, err := os.Stat(filepath.Join(umbrellaRoot, filepath.FromSlash(umbrella.PluginDirectory), forbidden)); err == nil {
				t.Errorf("the thin plugin holds %s", forbidden)
			}
		}
	})
}

// frontMatter splits a skill into its front matter lines and its body.
func frontMatter(t *testing.T, text string) (map[string]string, string) {
	t.Helper()
	rest, found := strings.CutPrefix(text, frontFence)
	if !found {
		t.Fatalf("no front matter in %q", text)
	}
	header, body, found := strings.Cut(rest, "\n"+frontFence)
	if !found {
		t.Fatalf("front matter is not closed in %q", text)
	}
	members := map[string]string{}
	for _, line := range strings.Split(header, "\n") {
		key, value, found := strings.Cut(line, ": ")
		if !found {
			t.Fatalf("not a key and value: %q", line)
		}
		members[key] = value
	}
	return members, body
}

func skillText(t *testing.T) (map[string]string, string) {
	t.Helper()
	return frontMatter(t, generated(t)[umbrella.PluginDirectory+"/skills/"+umbrella.SkillName+"/SKILL.md"])
}

func TestProveAllSkill(t *testing.T) {
	members, body := skillText(t)

	t.Run("the skill opens with the doctor of each module and names the modules that are missing [verifies SP-823~2]", func(t *testing.T) {
		if members["name"] != umbrella.SkillName || members["description"] == "" {
			t.Fatalf("members = %v", members)
		}
		firstStep := body[strings.Index(body, "\n1. Run "):strings.Index(body, "\n2. ")]
		for _, module := range umbrella.Modules {
			if !strings.Contains(firstStep, "`"+module.Binary+" doctor`") {
				t.Errorf("the first step does not run the doctor of %s", module.Binary)
			}
		}
		for _, want := range []string{"is not on `PATH` is missing", "name every missing module", "never counts as proved"} {
			if !strings.Contains(firstStep, want) {
				t.Errorf("the first step lacks %q", want)
			}
		}
	})

	t.Run("the modules run in the order discriminating power, alignment, completeness and their waves interleave [verifies SP-823~2]", func(t *testing.T) {
		order := body[strings.Index(body, "Modules, in this order"):strings.Index(body, "\n1. Run ")]
		previous := -1
		for _, module := range umbrella.Modules {
			index := strings.Index(order, "`"+module.Binary+"`")
			if index < 0 || index < previous {
				t.Fatalf("%s is out of order in:\n%s", module.Binary, order)
			}
			previous = index
		}
		for _, want := range []string{"in the order above", "before any module starts its next wave", "<binary> prove --changed", "<binary> prove --resume", "exit", "code 3"} {
			if !strings.Contains(body, want) {
				t.Errorf("the skill does not say %q", want)
			}
		}
	})

	t.Run("every request goes to the fulfil skill of its module and the skill never answers one itself [verifies SP-823~2]", func(t *testing.T) {
		for _, module := range umbrella.Modules {
			if !strings.Contains(body, "`"+module.FulfilSkill()+"`") {
				t.Errorf("the skill does not send the requests of %s to %s", module.Binary, module.FulfilSkill())
			}
		}
		for _, want := range []string{"holds no logic of its own", "Never answer a request yourself"} {
			if !strings.Contains(body, want) {
				t.Errorf("the skill does not say %q", want)
			}
		}
	})

	t.Run("the skill pre-approves only the doctor, the prove and the listing of each module, the relay's drain and the exact command that publishes the local proofs, never a command that answers a request [verifies SP-823~2]", func(t *testing.T) {
		allowed := strings.Split(members["allowed-tools"], ", ")
		var want []string
		for _, module := range umbrella.Modules {
			want = append(want, "Bash("+module.Binary+" doctor)", "Bash("+module.Binary+" prove *)", "Bash("+module.Binary+" request list)")
		}
		want = append(want, "Bash(spec-proof-s1 drain *)", "Bash(spec-proof-verify --accept-local .spec-proof/cache --out .spec-proof/verified)")
		for _, rule := range allowed {
			for _, answering := range []string{" request show", " request submit", " host ", " disable", " enable", " init", " *)"} {
				if strings.Contains(rule, answering) && !strings.HasSuffix(rule, " prove *)") && !strings.HasSuffix(rule, " drain *)") && !strings.HasPrefix(rule, "Bash(spec-proof-verify --accept-local ") {
					t.Errorf("%s pre-approves a command that answers or changes a project", rule)
				}
			}
		}
		if !slices.Equal(allowed, want) {
			t.Fatalf("allowed-tools = %v, want %v", allowed, want)
		}
	})

	t.Run("no generated file talks about its own authoring and every file ends with a newline [verifies SP-823~2]", func(t *testing.T) {
		forbidden := regexp.MustCompile(`(?i)\b(unverified|canary|ADR|previously|no longer|currently|this version|TODO)\b`)
		for path, content := range generated(t) {
			if found := forbidden.FindString(content); found != "" {
				t.Errorf("%s says %q", path, found)
			}
			if !strings.HasSuffix(content, "\n") || strings.Contains(content, " \n") {
				t.Errorf("%s is not tidy", path)
			}
		}
	})
}

func TestAgreementWithTheModuleTrees(t *testing.T) {
	modules := append(append([]umbrella.Module{}, umbrella.Modules...), umbrella.Relay)

	t.Run("each module's committed plugin bears the name the marketplace lists and holds the fulfil skill the umbrella names [verifies SP-824~1]", func(t *testing.T) {
		listed := map[string]bool{}
		for _, plugin := range readListing(t).Plugins {
			listed[plugin.Name] = true
		}
		for _, module := range modules {
			tree := filepath.Join(repositoryRoot, module.Directory, "plugin", "claude")
			content, err := os.ReadFile(filepath.Join(tree, filepath.FromSlash(manifestFile)))
			if err != nil {
				t.Fatalf("%s: %v", module.Plugin, err)
			}
			var manifest struct{ Name string }
			if err := json.Unmarshal(content, &manifest); err != nil {
				t.Fatal(err)
			}
			if manifest.Name != module.Plugin || !listed[manifest.Name] {
				t.Errorf("%s: the manifest names %q", module.Plugin, manifest.Name)
			}
		}
		for _, module := range umbrella.Modules {
			skill := filepath.Join(repositoryRoot, module.Directory, "plugin", "claude", "skills", module.FulfilSkill(), "SKILL.md")
			content, err := os.ReadFile(skill)
			if err != nil {
				t.Fatalf("%s: %v", module.Plugin, err)
			}
			members, _ := frontMatter(t, string(content))
			if members["name"] != module.FulfilSkill() {
				t.Errorf("%s: the skill is named %q", module.Plugin, members["name"])
			}
		}
	})

	t.Run("the hooks of each proof module run the binary the umbrella's skill runs [verifies SP-824~1]", func(t *testing.T) {
		for _, module := range umbrella.Modules {
			path := filepath.Join(repositoryRoot, module.Directory, "plugin", "claude", filepath.FromSlash(hooksFile))
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%s: %v", module.Plugin, err)
			}
			if !strings.Contains(string(content), `"command": "`+module.Binary+` hook `) {
				t.Errorf("%s: the hooks do not run %s", module.Plugin, module.Binary)
			}
			if strings.Contains(string(content), `"command": "spec-proof-`) && !strings.Contains(string(content), module.Binary) {
				t.Errorf("%s: the hooks run another module's binary", module.Plugin)
			}
		}
	})
}
