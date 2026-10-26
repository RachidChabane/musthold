package hostfiles_test

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/musthold/format/hostproto"
)

const codexHooksPath = ".codex/hooks.json"

func TestCodexHooksSelectTheSpawnTool(t *testing.T) {
	t.Run("the committed Codex pre-tool hook selects the spawn tool under each of its names and the executor tools [verifies SP-1938~1]", func(t *testing.T) {
		codex := hostharness.Codex()
		var hooks struct {
			Hooks map[string][]struct {
				Matcher string `json:"matcher"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal([]byte(committed(t, codex)[codexHooksPath]), &hooks); err != nil {
			t.Fatal(err)
		}
		groups := hooks.Hooks["PreToolUse"]
		if len(groups) != 1 {
			t.Fatalf("PreToolUse groups = %+v", groups)
		}
		matcher := regexp.MustCompile(groups[0].Matcher)
		for _, name := range append(hostproto.CodexSpawnToolNames(), codex.ToolNames()...) {
			if !matcher.MatchString(name) {
				t.Errorf("%s is not selected", name)
			}
		}
		for _, name := range []string{"Bash", "wait_agent", "multi_agent_v1wait_agent"} {
			if matcher.MatchString(name) {
				t.Errorf("%s is selected", name)
			}
		}
	})
}
