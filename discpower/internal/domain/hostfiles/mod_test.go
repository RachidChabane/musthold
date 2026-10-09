package hostfiles_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostfiles"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest/claudecli"
	"github.com/RachidChabane/spec-proof/format/hostproto/pluginfiles"
)

func TestClaudeMod(t *testing.T) {
	committedPlugin := filepath.Join(pluginDirectory, hostharness.FlagClaude)

	t.Run("the committed Claude plugin ships the mod of its own binary [verifies SP-1930~1]", func(t *testing.T) {
		source, err := os.ReadFile(filepath.Join(committedPlugin, filepath.FromSlash(pluginfiles.ClaudeModPath)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(source), `"`+hostfiles.Module.Binary+`"`) {
			t.Errorf("the mod does not name %s", hostfiles.Module.Binary)
		}
	})

	t.Run("claude plugin validate accepts the committed Claude plugin [verifies SP-1930~1]", func(t *testing.T) {
		claudecli.Validate(t, committedPlugin)
	})
}
