package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"slices"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// A Codex session keeps a rollout file per thread: the main agent's and each
// spawned agent's. `codex exec --json` does not print the call that spawns an
// agent, but the rollout of the main thread records it with its arguments, and
// the first line of a child's rollout names its parent, its role and its depth.
const (
	// RolloutMainFile is the rollout of the main thread, in the records directory.
	RolloutMainFile = "rollout-main.jsonl"
	// RolloutChildrenDir holds the rollouts of the threads the main thread spawned.
	RolloutChildrenDir = "rollout-children"

	// RolloutSpawnTool is the tool that starts an agent.
	RolloutSpawnTool = "spawn_agent"

	rolloutResponseItem       = "response_item"
	rolloutFunctionCall       = "function_call"
	rolloutFunctionCallOutput = "function_call_output"
	rolloutSessionMeta        = "session_meta"
)

// RolloutSpawn is one call of the spawn tool in the main thread's rollout.
type RolloutSpawn struct {
	// Line is the 1-based line of the call in RolloutMainFile.
	Line int
	// CallID ties the call to its output and to the hook payload of its tool use.
	CallID    string
	TaskName  string
	AgentType string
	// History holds the members of the call that decide how much of the main
	// agent's history the child receives, as compact JSON, by name; Blind is set
	// when they ask for none of it in a recognised blind form.
	History map[string]string
	Blind   bool
	// ChildThreadID and ChildPath name the thread the call started, as its output
	// gives it; both are empty when the call started none.
	ChildThreadID string
	ChildPath     string
}

// RolloutChild is the identity a spawned thread's rollout gives itself.
type RolloutChild struct {
	// File is the child's file name inside RolloutChildrenDir.
	File           string
	ThreadID       string
	ParentThreadID string
	AgentPath      string
	AgentRole      string
	Depth          int
	// Sandbox is the sandbox policy the thread ran under.
	Sandbox SandboxPolicy
	// History is what the rollout shows of the history the thread started with,
	// and HistoryReason why it is not blind.
	History       hostproto.ChildHistory
	HistoryReason string
}

// Rollouts is what the rollout files of a Codex session hold. Recorded is false
// when the session left none.
type Rollouts struct {
	Recorded bool
	Spawns   []RolloutSpawn
	Children []RolloutChild
}

// SpawnsOf returns the spawn calls whose agent type is the given one.
func (r Rollouts) SpawnsOf(agentType string) []RolloutSpawn {
	var found []RolloutSpawn
	for _, spawn := range r.Spawns {
		if spawn.AgentType == agentType {
			found = append(found, spawn)
		}
	}
	return found
}

// SpawnOf returns the spawn call that started a thread, and false when no
// recorded call names it.
func (r Rollouts) SpawnOf(child RolloutChild) (RolloutSpawn, bool) {
	for _, spawn := range r.Spawns {
		if (spawn.ChildThreadID != "" && spawn.ChildThreadID == child.ThreadID) || (spawn.ChildPath != "" && spawn.ChildPath == child.AgentPath) {
			return spawn, true
		}
	}
	return RolloutSpawn{}, false
}

// ChildrenOf returns the spawned threads whose role is the given one.
func (r Rollouts) ChildrenOf(role string) []RolloutChild {
	var found []RolloutChild
	for _, child := range r.Children {
		if child.AgentRole == role {
			found = append(found, child)
		}
	}
	return found
}

type rawRolloutLine struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type rawFunctionCall struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	CallID    string `json:"call_id"`
	Output    any    `json:"output"`
}

type rawSpawnArguments struct {
	TaskName  string `json:"task_name"`
	AgentType string `json:"agent_type"`
}

// rawSpawnOutput is what a spawn call returns: the started thread, by id or by
// task path depending on the toolset.
type rawSpawnOutput struct {
	AgentID  string `json:"agent_id"`
	TaskName string `json:"task_name"`
}

type rawSessionMeta struct {
	ID        string  `json:"id"`
	AgentRole *string `json:"agent_role"`
	Source    struct {
		Subagent struct {
			ThreadSpawn struct {
				ParentThreadID string  `json:"parent_thread_id"`
				Depth          int     `json:"depth"`
				AgentPath      string  `json:"agent_path"`
				AgentRole      *string `json:"agent_role"`
			} `json:"thread_spawn"`
		} `json:"subagent"`
	} `json:"source"`
}

// decodeRollouts decodes the main thread's rollout and the rollouts of its
// children, keyed by file name. A line that is not JSON is an error naming its file.
func decodeRollouts(main []byte, children map[string][]byte) (Rollouts, error) {
	spawns, err := decodeSpawns(main)
	if err != nil {
		return Rollouts{}, err
	}
	found := Rollouts{Recorded: len(main) > 0 || len(children) > 0, Spawns: spawns}
	names := make([]string, 0, len(children))
	for name := range children {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		child, err := decodeChild(name, children[name])
		if err != nil {
			return Rollouts{}, err
		}
		found.Children = append(found.Children, child)
	}
	return found, nil
}

// withoutIncompleteTail drops the last line of a rollout when it is not JSON and
// not ended by a newline: a session ended while it wrote leaves such a line, and
// it is no corruption of what came before it.
func withoutIncompleteTail(data []byte) []byte {
	if len(data) == 0 || data[len(data)-1] == '\n' {
		return data
	}
	complete := bytes.LastIndexByte(data, '\n') + 1
	if json.Valid(bytes.TrimSpace(data[complete:])) {
		return data
	}
	return data[:complete]
}

func decodeSpawns(data []byte) ([]RolloutSpawn, error) {
	lines, err := Lines(RolloutMainFile, withoutIncompleteTail(data))
	if err != nil {
		return nil, err
	}
	var spawns []RolloutSpawn
	outputs := map[string]rawSpawnOutput{}
	for _, line := range lines {
		var item rawRolloutLine
		var call rawFunctionCall
		if json.Unmarshal(line.Raw, &item) != nil || item.Type != rolloutResponseItem || json.Unmarshal(item.Payload, &call) != nil {
			continue
		}
		switch {
		case call.Type == rolloutFunctionCall && call.Name == RolloutSpawnTool:
			spawns = append(spawns, decodeSpawn(line.Number, call))
		case call.Type == rolloutFunctionCallOutput:
			outputs[call.CallID] = decodeSpawnOutput(call.Output)
		}
	}
	for index, spawn := range spawns {
		output := outputs[spawn.CallID]
		spawns[index].ChildThreadID, spawns[index].ChildPath = output.AgentID, output.TaskName
	}
	return spawns, nil
}

// decodeSpawn reads a spawn call: its agent type and task, and the history it
// asks for, judged by the blind forms of the host protocol.
func decodeSpawn(number int, call rawFunctionCall) RolloutSpawn {
	spawn := RolloutSpawn{Line: number, CallID: call.CallID}
	var arguments rawSpawnArguments
	_ = json.Unmarshal([]byte(call.Arguments), &arguments)
	spawn.TaskName, spawn.AgentType = arguments.TaskName, arguments.AgentType
	if input, err := hostproto.DecodeCodexSpawn(json.RawMessage(call.Arguments)); err == nil {
		spawn.History, spawn.Blind = input.HistoryControls(), input.RequestsNoHistory()
	}
	return spawn
}

// decodeSpawnOutput reads the thread a spawn started out of its output, a string
// that holds a JSON document; anything else names no thread.
func decodeSpawnOutput(output any) rawSpawnOutput {
	var started rawSpawnOutput
	if text, ok := output.(string); ok {
		_ = json.Unmarshal([]byte(text), &started)
	}
	return started
}

func decodeChild(name string, data []byte) (RolloutChild, error) {
	file := path.Join(RolloutChildrenDir, name)
	lines, err := Lines(file, withoutIncompleteTail(data))
	if err != nil {
		return RolloutChild{}, err
	}
	judged := hostproto.ReadCodexChildRollout(data)
	child := RolloutChild{File: name, Sandbox: decodeSandboxPolicy(lines), History: judged.History, HistoryReason: judged.Reason}
	for _, line := range lines {
		var item rawRolloutLine
		if json.Unmarshal(line.Raw, &item) != nil || item.Type != rolloutSessionMeta {
			continue
		}
		var meta rawSessionMeta
		if err := json.Unmarshal(item.Payload, &meta); err != nil {
			return RolloutChild{}, fmt.Errorf("%s: %w", file, err)
		}
		spawn := meta.Source.Subagent.ThreadSpawn
		child.ThreadID, child.ParentThreadID, child.AgentPath, child.Depth = meta.ID, spawn.ParentThreadID, spawn.AgentPath, spawn.Depth
		child.AgentRole = firstRole(spawn.AgentRole, meta.AgentRole)
		break
	}
	return child, nil
}

func firstRole(roles ...*string) string {
	for _, role := range roles {
		if role != nil && *role != "" {
			return *role
		}
	}
	return ""
}

// RolloutParent reads the first line of a rollout and returns the thread it was
// spawned from; ok is false for a rollout that is not a spawned thread's.
func RolloutParent(firstLine []byte) (parentThreadID string, ok bool) {
	var item rawRolloutLine
	if json.Unmarshal(firstLine, &item) != nil || item.Type != rolloutSessionMeta {
		return "", false
	}
	var meta rawSessionMeta
	if json.Unmarshal(item.Payload, &meta) != nil {
		return "", false
	}
	parent := meta.Source.Subagent.ThreadSpawn.ParentThreadID
	return parent, parent != ""
}
