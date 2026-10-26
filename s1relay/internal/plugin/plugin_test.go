package plugin_test

import (
	"encoding/json"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/s1relay/internal/plugin"
)

// update rewrites the committed tree from the generator.
var update = flag.Bool("update", false, "rewrite the committed plugin tree from the generator")

const committedTree = "../../plugin/claude"

func generated(t *testing.T) map[string]string {
	t.Helper()
	files, err := plugin.For()
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

func committed(t *testing.T) map[string]string {
	t.Helper()
	byPath := map[string]string{}
	err := filepath.WalkDir(committedTree, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(committedTree, path)
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

func TestCommittedTreeIsGenerated(t *testing.T) {
	t.Run("the committed tree is what the generator produces, byte for byte, and generating twice gives the same bytes [verifies SP-830~1]", func(t *testing.T) {
		want := generated(t)
		if *update {
			if err := os.RemoveAll(committedTree); err != nil {
				t.Fatal(err)
			}
			for path, content := range want {
				target := filepath.Join(committedTree, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		got := committed(t)
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

func TestPluginContract(t *testing.T) {
	files := generated(t)

	t.Run("the plugin has a manifest naming it and one skill that runs the relay for each module [verifies SP-831~1]", func(t *testing.T) {
		var manifest struct {
			Name, Version, Description, License string
			Author                              map[string]string
		}
		if err := json.Unmarshal([]byte(files[".claude-plugin/plugin.json"]), &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Name != plugin.Name || !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(manifest.Version) || manifest.Description == "" {
			t.Fatalf("manifest = %+v", manifest)
		}
		if len(files) != 2 {
			t.Fatalf("files = %d, want a manifest and one skill", len(files))
		}
		skill := files["skills/"+plugin.SkillName+"/SKILL.md"]
		for _, want := range []string{
			"name: " + plugin.SkillName, "allowed-tools: Bash(" + plugin.Binary + " drain *)",
			plugin.Binary + " drain --module <module>", "`dp`, `alignment` or `completeness`", "systemone.endpoint",
		} {
			if !strings.Contains(skill, want) {
				t.Errorf("the skill lacks %q:\n%s", want, skill)
			}
		}
	})

	t.Run("no generated file talks about its own authoring and every file ends with a newline [verifies SP-831~1]", func(t *testing.T) {
		forbidden := regexp.MustCompile(`(?i)\b(unverified|canary|ADR|previously|no longer|currently|this version|TODO)\b`)
		for path, content := range files {
			if found := forbidden.FindString(content); found != "" {
				t.Errorf("%s says %q", path, found)
			}
			if !strings.HasSuffix(content, "\n") || strings.Contains(content, " \n") {
				t.Errorf("%s is not tidy", path)
			}
		}
	})
}
