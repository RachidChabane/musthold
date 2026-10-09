package hostproto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// Codex starts an agent with its `spawn_agent` tool. A hook payload names the
// tool bare, or prefixed by the namespace of the collaboration toolset that
// offers it.
const (
	// CodexSpawnTool is the bare name of the tool that starts an agent.
	CodexSpawnTool = "spawn_agent"
	// codexCollaborationNamespace and codexMultiAgentNamespace are the namespaces
	// a hook payload may prefix the spawn tool with.
	codexCollaborationNamespace = "collaboration"
	codexMultiAgentNamespace    = "multi_agent_v1"
)

// Members of a `spawn_agent` input that decide how much of the spawning agent's
// history the child receives.
const (
	// CodexForkTurns is the number of the spawning agent's turns the child
	// receives: "none", "all" or a count.
	CodexForkTurns = "fork_turns"
	// CodexForkContext tells whether the child receives the spawning agent's context.
	CodexForkContext = "fork_context"
)

// codexSpawnNamespaces are the prefixes a hook payload may give the spawn tool,
// the empty one first.
var codexSpawnNamespaces = []string{"", codexCollaborationNamespace, codexMultiAgentNamespace}

// CodexSpawnToolNames lists every name a Codex hook payload gives the spawn tool.
func CodexSpawnToolNames() []string {
	names := make([]string, len(codexSpawnNamespaces))
	for index, namespace := range codexSpawnNamespaces {
		names[index] = namespace + CodexSpawnTool
	}
	return names
}

// IsCodexSpawnTool reports whether a hook payload's tool name is the spawn tool.
func IsCodexSpawnTool(toolName string) bool {
	return slices.Contains(CodexSpawnToolNames(), toolName)
}

// BlindForm is one way a spawn asks for a child with none of the spawning
// agent's history: the member and its value, as compact JSON.
type BlindForm struct {
	Member string
	Value  string
}

// codexBlindForms are the spawn inputs that give the child none of the spawning
// agent's history.
var codexBlindForms = []BlindForm{
	{Member: CodexForkTurns, Value: `"none"`},
	{Member: CodexForkContext, Value: `false`},
}

// CodexBlindForms lists the recognised blind forms of a spawn.
func CodexBlindForms() []BlindForm { return slices.Clone(codexBlindForms) }

// codexHistoryMembers are the members of a spawn input that decide the child's
// history.
var codexHistoryMembers = []string{CodexForkTurns, CodexForkContext}

// CodexSpawn is the input of a `spawn_agent` call, as far as the module reads it.
type CodexSpawn struct {
	// AgentType is the role file the child runs as.
	AgentType string
	// Message is the task the child is given.
	Message string
	// history holds the members of the input that decide the child's history, as
	// compact JSON, by name.
	history map[string]string
}

// DecodeCodexSpawn reads the input of a `spawn_agent` call.
func DecodeCodexSpawn(raw json.RawMessage) (CodexSpawn, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil || members == nil {
		return CodexSpawn{}, fmt.Errorf("%w: spawn input is not an object", ErrBadHookPayload)
	}
	spawn := CodexSpawn{history: map[string]string{}}
	for name, target := range map[string]*string{"agent_type": &spawn.AgentType, "message": &spawn.Message} {
		if value, present := members[name]; present {
			_ = json.Unmarshal(value, target)
		}
	}
	for _, member := range codexHistoryMembers {
		value, present := members[member]
		if !present {
			continue
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, value); err != nil {
			return CodexSpawn{}, fmt.Errorf("%w: %s: %v", ErrBadHookPayload, member, err)
		}
		spawn.history[member] = compact.String()
	}
	return spawn, nil
}

// HistoryControls returns the members of the input that decide the child's
// history, as compact JSON, by name.
func (s CodexSpawn) HistoryControls() map[string]string {
	controls := make(map[string]string, len(s.history))
	for member, value := range s.history {
		controls[member] = value
	}
	return controls
}

// RequestsNoHistory reports whether the spawn asks for a child with none of the
// spawning agent's history: it sets at least one member that decides the
// history, and every such member it sets holds a recognised blind form. A spawn
// that sets none of them gives the child the whole history.
func (s CodexSpawn) RequestsNoHistory() bool {
	if len(s.history) == 0 {
		return false
	}
	for member, value := range s.history {
		if !slices.Contains(codexBlindForms, BlindForm{Member: member, Value: value}) {
			return false
		}
	}
	return true
}

// CodexSpawnDenial is the reason the pre-tool hook gives when it refuses a spawn
// of an executor that does not ask for a blind child.
func CodexSpawnDenial(agentType string) string {
	return fmt.Sprintf("an executor must start with none of your history: call %s again for %s with the same arguments, %s set to %s and no %s",
		CodexSpawnTool, agentType, CodexForkTurns, codexBlindForms[0].Value, CodexForkContext)
}
