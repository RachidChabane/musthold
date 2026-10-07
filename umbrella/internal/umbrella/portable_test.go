package umbrella_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto/pluginfiles"
	"github.com/RachidChabane/spec-proof/umbrella/internal/umbrella"
)

const portableTree = "../../" + umbrella.PortableDirectory

func portableGenerated(t *testing.T) map[string]string {
	t.Helper()
	files, err := umbrella.PortableFiles()
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]string{}
	for _, file := range files {
		byPath[file.Path] = string(file.Content)
	}
	return byPath
}

func portableCommitted(t *testing.T) map[string]string {
	t.Helper()
	byPath := map[string]string{}
	err := filepath.WalkDir(portableTree, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(portableTree, path)
		byPath[filepath.ToSlash(relative)] = string(content)
		return err
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return byPath
}

func TestPortableTreeIsGenerated(t *testing.T) {
	t.Run("the committed portable tree is what the generator produces, byte for byte [verifies SP-1706~1]", func(t *testing.T) {
		want := portableGenerated(t)
		if *update {
			if err := os.RemoveAll(portableTree); err != nil {
				t.Fatal(err)
			}
			for path, content := range want {
				target := filepath.Join(portableTree, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		got := portableCommitted(t)
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
	})

	t.Run("the portable umbrella holds a manifest, an empty server list and the prove-all skill [verifies SP-1706~1]", func(t *testing.T) {
		files := portableGenerated(t)
		var paths []string
		for path := range files {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		want := []string{pluginfiles.PortableMCPPath, pluginfiles.PortableManifestPath, pluginfiles.PortableSkillPath(umbrella.SkillName)}
		slices.Sort(want)
		if !slices.Equal(paths, want) {
			t.Fatalf("files = %v, want %v", paths, want)
		}
		if !strings.Contains(files[pluginfiles.PortableMCPPath], `"mcpServers": {}`) {
			t.Errorf("mcp.json:\n%s", files[pluginfiles.PortableMCPPath])
		}
		skill := files[pluginfiles.PortableSkillPath(umbrella.SkillName)]
		if strings.Contains(skill, "allowed-tools") || strings.Contains(skill, "blind executor") {
			t.Errorf("skill:\n%s", skill)
		}
		for _, module := range umbrella.Modules {
			if !strings.Contains(skill, module.FulfilSkill()) {
				t.Errorf("the skill does not name the loop %s", module.FulfilSkill())
			}
		}
	})
}
