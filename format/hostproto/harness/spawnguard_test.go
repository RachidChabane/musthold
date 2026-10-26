package harness_test

import (
	"regexp"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/harness"
)

func TestSpawnGuard(t *testing.T) {
	t.Run("Codex alone guards spawns and reads the stop transcript as a rollout [verifies SP-1937~1]", func(t *testing.T) {
		want := map[string]hostproto.TranscriptFormat{
			harness.FlagClaude:  hostproto.TranscriptAgentJSONL,
			harness.FlagCodex:   hostproto.TranscriptCodexRollout,
			harness.FlagCopilot: hostproto.TranscriptCopilotSession,
		}
		for _, h := range harness.All(sample) {
			if h.Behaviours.GuardsSpawn != (h.Flag == harness.FlagCodex) || h.Transcript != want[h.Flag] {
				t.Errorf("%s: guards %v, transcript %q", h.Flag, h.Behaviours.GuardsSpawn, h.Transcript)
			}
		}
	})

	t.Run("the Codex pre-tool matcher selects the spawn tool under each of its names and the executor tools, and nothing else [verifies SP-1937~1]", func(t *testing.T) {
		codex := harness.Codex(sample)
		matcher := regexp.MustCompile(codex.PreToolMatcher())
		for _, name := range append(hostproto.CodexSpawnToolNames(), codex.ToolNames()...) {
			if !matcher.MatchString(name) {
				t.Errorf("%s is not selected", name)
			}
		}
		for _, name := range []string{"Bash", "wait_agent", "multi_agent_v1wait_agent", "spawn_agent_x", "my_spawn_agent", "mcp__musthold_other_exec__fetch_request"} {
			if matcher.MatchString(name) {
				t.Errorf("%s is selected", name)
			}
		}
	})

	t.Run("a harness that does not guard spawns keeps the matcher of its executor tools [verifies SP-1937~1]", func(t *testing.T) {
		for _, h := range []harness.Harness{harness.Claude(sample), harness.Copilot(sample)} {
			if h.PreToolMatcher() != h.ToolMatcher() {
				t.Errorf("%s: matcher = %q", h.Flag, h.PreToolMatcher())
			}
		}
	})
}
