package hostfiles_test

import (
	"regexp"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostfiles"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// agentTypes lists the type each harness gives the generated executor agents: the
// name of the agent as its file declares it, joined with the plugin where the
// harness scopes agents by plugin.
func agentTypes(t *testing.T, harness hostharness.Harness, files map[string]string) []string {
	t.Helper()
	var types []string
	for _, agent := range hostproto.Agents() {
		switch harness.Flag {
		case hostharness.FlagClaude:
			members, _ := frontMatter(t, files["agents/"+string(agent)+".md"])
			types = append(types, hostharness.PluginName+":"+members["name"].(string))
		case hostharness.FlagCodex:
			types = append(types, tomlTable(t, files[".codex/agents/spec-proof-dp-"+string(agent)+".toml"])[""]["name"].(string))
		case hostharness.FlagCopilot:
			members, _ := frontMatter(t, files[".github/agents/spec-proof-dp-"+string(agent)+".agent.md"])
			types = append(types, members["name"].(string))
		}
	}
	return types
}

func TestAgentNamesMeetTheHooksAndTheRoles(t *testing.T) {
	t.Run("the type of each generated agent passes the matchers of the generated hooks and maps to the role of its requests [verifies SP-242~1]", func(t *testing.T) {
		hooksFiles := map[string]string{hostharness.FlagClaude: "hooks/hooks.json", hostharness.FlagCodex: ".codex/hooks.json"}
		for _, harness := range hostharness.All() {
			files := generated(t, harness)
			for index, agentType := range agentTypes(t, harness, files) {
				role, ok := harness.RoleOf(agentType)
				if want := hostproto.RoleFor(hostharness.PluginName, hostproto.Agents()[index]); !ok || role != want {
					t.Errorf("%s: %q maps to %q, %v, want %q", harness.Flag, agentType, role, ok, want)
				}
				hooksFile, hasMatchers := hooksFiles[harness.Flag]
				if !hasMatchers {
					continue
				}
				var parsed snakeHooksFile
				decodeJSON(t, files[hooksFile], &parsed)
				for _, event := range []string{"SubagentStart", "SubagentStop"} {
					if !regexp.MustCompile(parsed.Hooks[event][0].Matcher).MatchString(agentType) {
						t.Errorf("%s: the %s matcher %q does not select the agent type %q", harness.Flag, event, parsed.Hooks[event][0].Matcher, agentType)
					}
				}
			}
		}
	})

	t.Run("the matcher of a harness selects its own agent types and not the form of another [verifies SP-242~1]", func(t *testing.T) {
		claude := regexp.MustCompile(hostfiles.ExecutorRoleMatcher(hostharness.Claude()))
		codex := regexp.MustCompile(hostfiles.ExecutorRoleMatcher(hostharness.Codex()))
		if claude.MatchString("spec-proof-dp-judge") || codex.MatchString("spec-proof-dp:judge") {
			t.Error("a matcher selects the form of another harness")
		}
		if !claude.MatchString("spec-proof-dp:judge") || !codex.MatchString("spec-proof-dp-judge") || codex.MatchString("spec-proof-dp-judgey") {
			t.Error("a matcher misses its own form")
		}
	})
}
