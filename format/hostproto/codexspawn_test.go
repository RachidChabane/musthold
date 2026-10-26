package hostproto_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
)

const deciderType = "musthold-dp-decider"

func spawnInput(t *testing.T, members map[string]any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(members)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestCodexSpawnTool(t *testing.T) {
	t.Run("the spawn tool is recognized bare and under the namespaces of the collaboration toolsets, and no other tool is [verifies SP-1933~1]", func(t *testing.T) {
		for _, name := range []string{"spawn_agent", "collaborationspawn_agent", "multi_agent_v1spawn_agent"} {
			if !hostproto.IsCodexSpawnTool(name) {
				t.Errorf("%s is not recognized", name)
			}
		}
		for _, name := range []string{"wait_agent", "multi_agent_v1wait_agent", "spawn_agent_x", "Agent", "mcp__musthold_dp_exec__fetch_request", ""} {
			if hostproto.IsCodexSpawnTool(name) {
				t.Errorf("%s is recognized", name)
			}
		}
	})
}

func TestCodexBlindForms(t *testing.T) {
	t.Run("the blind forms are fork_turns none and fork_context false [verifies SP-1933~1]", func(t *testing.T) {
		want := []hostproto.BlindForm{{Member: "fork_turns", Value: `"none"`}, {Member: "fork_context", Value: `false`}}
		got := hostproto.CodexBlindForms()
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("forms = %+v", got)
		}
	})

	t.Run("a spawn asks for a blind child only when every history member it sets holds a blind form [verifies SP-1933~1]", func(t *testing.T) {
		for name, test := range map[string]struct {
			members map[string]any
			blind   bool
		}{
			"fork_turns none":                     {map[string]any{"fork_turns": "none"}, true},
			"fork_context false":                  {map[string]any{"fork_context": false}, true},
			"both blind":                          {map[string]any{"fork_turns": "none", "fork_context": false}, true},
			"no history member":                   {map[string]any{}, false},
			"fork_turns all":                      {map[string]any{"fork_turns": "all"}, false},
			"fork_turns a count":                  {map[string]any{"fork_turns": 3}, false},
			"fork_turns None spelled otherwise":   {map[string]any{"fork_turns": "None"}, false},
			"fork_context true":                   {map[string]any{"fork_context": true}, false},
			"fork_context the string false":       {map[string]any{"fork_context": "false"}, false},
			"blind turns but forked context":      {map[string]any{"fork_turns": "none", "fork_context": true}, false},
			"blind context but all of the turns":  {map[string]any{"fork_turns": "all", "fork_context": false}, false},
			"fork_turns null":                     {map[string]any{"fork_turns": nil}, false},
			"blind form beside an unknown member": {map[string]any{"fork_turns": "none", "fork_depth": 2}, true},
		} {
			members := map[string]any{"agent_type": deciderType, "message": "ticket t"}
			for key, value := range test.members {
				members[key] = value
			}
			spawn, err := hostproto.DecodeCodexSpawn(spawnInput(t, members))
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if spawn.AgentType != deciderType || spawn.Message != "ticket t" {
				t.Errorf("%s: spawn = %+v", name, spawn)
			}
			if got := spawn.RequestsNoHistory(); got != test.blind {
				t.Errorf("%s: blind = %v, want %v", name, got, test.blind)
			}
		}
	})

	t.Run("the history members are read as compact JSON whatever their spacing [verifies SP-1933~1]", func(t *testing.T) {
		spawn, err := hostproto.DecodeCodexSpawn(json.RawMessage(`{"agent_type":"x","fork_turns" :  "none" ,"fork_context": false }`))
		if err != nil || !spawn.RequestsNoHistory() {
			t.Fatalf("spawn %+v, error %v", spawn, err)
		}
		if controls := spawn.HistoryControls(); controls["fork_turns"] != `"none"` || controls["fork_context"] != "false" {
			t.Fatalf("controls = %v", controls)
		}
	})

	t.Run("an input that is not an object is refused [verifies SP-1933~1]", func(t *testing.T) {
		for _, raw := range []string{``, `null`, `[]`, `"spawn"`, `{`} {
			if _, err := hostproto.DecodeCodexSpawn(json.RawMessage(raw)); !errors.Is(err, hostproto.ErrBadHookPayload) {
				t.Errorf("%q: error %v", raw, err)
			}
		}
	})

	t.Run("the denial tells the agent to spawn again with fork_turns none [verifies SP-1933~1]", func(t *testing.T) {
		reason := hostproto.CodexSpawnDenial(deciderType)
		for _, part := range []string{"spawn_agent", deciderType, `fork_turns set to "none"`, "no fork_context"} {
			if !strings.Contains(reason, part) {
				t.Errorf("the reason lacks %q: %s", part, reason)
			}
		}
	})
}
