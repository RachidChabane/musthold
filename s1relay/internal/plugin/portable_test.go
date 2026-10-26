package plugin_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto/pluginfiles"
	"github.com/RachidChabane/musthold/s1relay/internal/plugin"
)

const portableTree = "../../plugin/portable"

func portableGenerated(t *testing.T) map[string]string {
	t.Helper()
	files, err := plugin.Portable()
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
	t.Run("the committed portable tree is what the generator produces, byte for byte [verifies SP-1705~1]", func(t *testing.T) {
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

	t.Run("the portable plugin of the relay holds a manifest and a skill and no MCP server [verifies SP-1705~1]", func(t *testing.T) {
		files := portableGenerated(t)
		var paths []string
		for path := range files {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		if want := []string{pluginfiles.PortableManifestPath, pluginfiles.PortableSkillPath(plugin.SkillName)}; !slices.Equal(paths, want) {
			t.Fatalf("files = %v, want %v", paths, want)
		}
		skill := files[pluginfiles.PortableSkillPath(plugin.SkillName)]
		if strings.Contains(skill, "allowed-tools") || !strings.Contains(skill, "musthold-s1 drain --module <module>") {
			t.Errorf("skill:\n%s", skill)
		}
	})
}
