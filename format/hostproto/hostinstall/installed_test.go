package hostinstall_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto/harness"
)

func TestInstalled(t *testing.T) {
	eachModule(t, "the harnesses a project holds the module's files for are listed", "[verifies SP-717~2]", func(t *testing.T, s sample) {
		files := repository(t)
		if got := s.module.Installed(files); len(got) != 0 {
			t.Fatalf("a project without the module's files lists %v", got)
		}
		apply(t, files)(initPlan(s, files, s.harness(harness.FlagClaude), s.harness(harness.FlagCodex), s.harness(harness.FlagCopilot)))
		got := s.module.Installed(files)
		slices.Sort(got)
		if want := []string{harness.FlagClaude, harness.FlagCodex, harness.FlagCopilot}; !slices.Equal(got, want) {
			t.Fatalf("installed = %v, want %v", got, want)
		}
	})

	eachModule(t, "a plugin another module enabled is not this module's", "[verifies SP-717~2]", func(t *testing.T, s sample) {
		files := repository(t)
		files.put(t, settingsPath, `{"enabledPlugins":{"musthold-other@musthold":true}}`)
		files.put(t, ".codex/hooks.json", `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"shallnot hook stop"}]}]}}`)
		if got := s.module.Installed(files); len(got) != 0 {
			t.Fatalf("installed = %v", got)
		}
	})
}
