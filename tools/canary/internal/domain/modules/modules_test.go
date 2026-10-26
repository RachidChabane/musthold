package modules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto/harness"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/modules"
)

// repoRoot is the repository root, seen from this package.
const repoRoot = "../../../../.."

// skillFile is where a plugin tree keeps its skill.
const skillFile = "SKILL.md"

func TestModulesMatchTheirShippedPlugins(t *testing.T) {
	for _, module := range modules.All() {
		t.Run(module.Dir+" is described as its shipped Claude Code plugin is generated, so the AGENTS.md block rendered from it is the module's [verifies SP-1943~1]", func(t *testing.T) {
			files, err := module.Files().For(harness.Claude(module.Identity()))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, file := range files {
				if !strings.HasSuffix(file.Path, "/"+skillFile) {
					continue
				}
				found = true
				committed, err := os.ReadFile(filepath.Join(repoRoot, module.PluginDir(harness.FlagClaude), file.Path))
				if err != nil {
					t.Fatal(err)
				}
				if string(committed) != string(file.Content) {
					t.Errorf("%s differs from the committed skill:\n%s", file.Path, file.Content)
				}
			}
			if !found {
				t.Fatal("the generated plugin has no skill")
			}
		})
	}
}
