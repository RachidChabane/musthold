package checks

import (
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

// Names of the Codex checks.
const (
	checkCodexAgentLoads  = "agent_files_load"
	checkCodexSpawnBlind  = "executor_spawn_blind"
	checkCodexHooksFire   = "hooks_fire"
	checkCodexAgentType   = "agent_type_in_pretool"
	checkCodexToolNaming  = "mcp_tool_naming"
	checkCodexSandbox     = "executor_sandbox"
	checkCodexNoSpawnNote = "the session spawned no executor agent of the primary module"
)

func codexStrategy() harness.Harness { return strategy(harness.FlagCodex) }

func codexChecks() []Check {
	return []Check{
		versionCheck(harness.FlagCodex),
		capWithoutHooksCheck(harness.FlagCodex),
		serversStartCheck(),
		agentFilesLoadCheck(),
		executorSpawnBlindCheck(),
		hooksFireCodexCheck(),
		agentTypeInPreToolCheck(),
		toolNamingCheck(),
		sandboxCheck(),
		stopProbeCheck(stopSources{finished: codexFinished, streamStops: func(evidence.Evidence) []verdict.Pointer { return nil }, controls: codexControls}),
		editAppliedCheck(),
		withinCapsCheck(codexSummary),
		configUntouchedCheck(),
	}
}

func codexStream(found evidence.Evidence) evidence.CodexStream {
	return evidence.DecodeCodex(found.Stream)
}

func codexFinished(found evidence.Evidence) (bool, verdict.Pointer) {
	stream := codexStream(found)
	for index, kind := range stream.Types {
		if kind == evidence.CodexEventTurnCompleted {
			return !slices.Contains(stream.Types, evidence.CodexEventTurnFailed), evidence.StreamAt(found.Stream[index].Number, "turn.completed")
		}
	}
	return false, verdict.Pointer{}
}

// codexControls are the hooks of the project's hooks file other than Stop that
// ran: the probe that records every PreToolUse payload.
func codexControls(found evidence.Evidence) []verdict.Pointer {
	var pointers []verdict.Pointer
	for _, probe := range found.Probes(evidence.LabelPreToolProbe) {
		pointers = append(pointers, evidence.HooksAt(probe.Line, evidence.LabelPreToolProbe))
	}
	return pointers
}

func codexSummary(found evidence.Evidence) (string, bool, verdict.Pointer) {
	finished, pointer := codexFinished(found)
	if !finished {
		return "", false, verdict.Pointer{}
	}
	return fmt.Sprintf("the turn completed after %d events", len(found.Stream)), true, pointer
}

// executorSpawn is one start of the executor agent, as the recorded session shows it.
type executorSpawn struct {
	Pointer verdict.Pointer
	// Failed says the harness reported the start as failed.
	Failed bool
	Detail string
}

// spawnedExecutor returns the starts of the executor's agent type and the agent
// type. The rollouts are the primary source: the spawn calls of the main thread
// whose agent type is the executor's, and the threads whose role is. The collab
// items of the event stream stand in when the session left no rollout, and report
// the starts that failed whatever the rollouts hold.
func spawnedExecutor(found evidence.Evidence) ([]executorSpawn, string) {
	agentType := codexStrategy().AgentType(found.Session.Role)
	var spawns []executorSpawn
	for _, call := range found.Rollouts.SpawnsOf(agentType) {
		spawns = append(spawns, executorSpawn{Pointer: evidence.At(evidence.RolloutMainFile, call.Line, evidence.RolloutSpawnTool)})
	}
	for _, child := range found.Rollouts.ChildrenOf(agentType) {
		spawns = append(spawns, executorSpawn{Pointer: evidence.At(evidence.RolloutChildrenDir+"/"+child.File, 1, "session_meta")})
	}
	for _, item := range codexStream(found).ItemsOf(evidence.CodexItemCollab) {
		if !item.Mentions(agentType) {
			continue
		}
		failed := item.Status == evidence.CodexStatusFailed
		if failed || !found.Rollouts.Recorded {
			spawns = append(spawns, executorSpawn{Pointer: evidence.StreamAt(item.Line, "collab call"), Failed: failed, Detail: truncate(string(item.Raw))})
		}
	}
	return spawns, agentType
}

func agentFilesLoadCheck() Check {
	return newCheck(checkCodexAgentLoads,
		"The custom agent files and the plugin's skill load under `codex exec`: the harness starts an agent of the module's type.",
		func(found evidence.Evidence) verdict.Verdict {
			spawns, agentType := spawnedExecutor(found)
			for _, call := range found.HookCalls(primaryModule().Binary, hookKindStart) {
				if call.Payload().AgentType == agentType {
					return verdict.Passed("SubagentStart fired for agent type "+agentType, evidence.HooksAt(call.Line, "subagent-start"))
				}
			}
			for _, spawn := range spawns {
				if spawn.Failed {
					return verdict.Failed("the spawn of "+agentType+" failed: "+spawn.Detail, spawn.Pointer)
				}
			}
			if len(spawns) > 0 {
				return verdict.Undecided("a spawn of "+agentType+" was attempted but no SubagentStart hook recorded it", spawns[0].Pointer)
			}
			return verdict.Undecided(checkCodexNoSpawnNote)
		})
}

func hooksFireCodexCheck() Check {
	required := []string{hookKindStart, hookKindStop, hookKindPreTool}
	return newCheck(checkCodexHooksFire,
		"Project hooks of the modules run under `codex exec` once the project is trusted: subagent start, subagent stop and the executor tool guard, each exiting cleanly.",
		func(found evidence.Evidence) verdict.Verdict {
			binary := primaryModule().Binary
			if !slices.ContainsFunc(found.Hooks, func(call evidence.HookCall) bool { return call.Module == binary }) {
				if spawns, _ := spawnedExecutor(found); len(spawns) == 0 {
					return verdict.Undecided(checkCodexNoSpawnNote)
				}
				return verdict.Undecided("the recorder saw no command of " + binary + ": hooks did not run, or the recorder is not on the PATH of the session")
			}
			var missing []string
			var pointers []verdict.Pointer
			unexercised := false
			for _, kind := range required {
				calls := found.HookCalls(binary, kind)
				if len(calls) == 0 && kind == hookKindPreTool && !executorToolCalled(found) {
					unexercised = true
					continue
				}
				if len(calls) == 0 {
					missing = append(missing, kind)
					continue
				}
				for _, call := range calls {
					if call.ExitCode != 0 {
						return verdict.Failed(fmt.Sprintf("the %s hook exited %d", kind, call.ExitCode), evidence.HooksAt(call.Line, kind))
					}
				}
				pointers = append(pointers, evidence.HooksAt(calls[0].Line, kind))
			}
			if len(missing) > 0 {
				return verdict.Failed("hooks that never ran: "+strings.Join(missing, ", "), pointers...)
			}
			if unexercised {
				return verdict.Undecided("the pre-tool hook never ran, but no executor tool call was made for it to match", pointers...)
			}
			return verdict.Passed("the subagent start, subagent stop and pre-tool hooks ran and exited 0", pointers...)
		})
}

// executorToolCalled reports whether the session called an executor tool, as a
// PreToolUse payload of any spelling that names one, or as an MCP call the event
// stream records.
func executorToolCalled(found evidence.Evidence) bool {
	for _, call := range preToolRecords(found) {
		name := call.Payload().ToolName
		if strings.Contains(name, hostproto.ToolFetchRequest) || strings.Contains(name, hostproto.ToolSubmitResponse) {
			return true
		}
	}
	for _, item := range codexStream(found).ItemsOf(evidence.CodexItemMCPCall) {
		if item.Tool == hostproto.ToolFetchRequest || item.Tool == hostproto.ToolSubmitResponse {
			return true
		}
	}
	return false
}

// preToolRecords returns every recorded PreToolUse payload: the module's own hook
// and the probe that sees every tool call whatever the module's matcher selects.
func preToolRecords(found evidence.Evidence) []evidence.HookCall {
	records := found.HookCalls(primaryModule().Binary, hookKindPreTool)
	return append(records, found.Probes(evidence.LabelPreToolProbe)...)
}

// executorToolCalls returns the PreToolUse records whose tool is an executor tool
// as the harness spells it.
func executorToolCalls(found evidence.Evidence) []evidence.HookCall {
	var calls []evidence.HookCall
	for _, call := range preToolRecords(found) {
		if _, executor := codexStrategy().ExecutorTool(call.Payload().ToolName); executor {
			calls = append(calls, call)
		}
	}
	return calls
}

func agentTypeInPreToolCheck() Check {
	return newCheck(checkCodexAgentType,
		"PreToolUse names the calling agent: the payload of an executor tool call carries the agent id and the agent type, so a hook can tell the executor from the main agent.",
		func(found evidence.Evidence) verdict.Verdict {
			calls := executorToolCalls(found)
			if len(calls) == 0 {
				return verdict.Undecided("no PreToolUse payload for an executor tool was recorded")
			}
			want := codexStrategy().AgentType(found.Session.Role)
			for _, call := range calls {
				payload := call.Payload()
				if payload.AgentType == want && payload.AgentID != "" {
					return verdict.Passed(fmt.Sprintf("the payload names agent_type %q and an agent_id", want), evidence.HooksAt(call.Line, "pre-tool payload"))
				}
			}
			first := calls[0].Payload()
			return verdict.Failed(fmt.Sprintf("the payload carries agent_type %q and agent_id %q, expected %q: a hook cannot attribute the call", first.AgentType, first.AgentID, want), evidence.HooksAt(calls[0].Line, "pre-tool payload"))
		})
}

func toolNamingCheck() Check {
	codex := codexStrategy()
	return newCheck(checkCodexToolNaming,
		"A hook sees the module's MCP tools under the name the module's matchers and allowlists spell, so the matcher selects them.",
		func(found evidence.Evidence) verdict.Verdict {
			for _, call := range preToolRecords(found) {
				name := call.Payload().ToolName
				pointer := evidence.HooksAt(call.Line, "pre-tool payload")
				if tool, executor := codex.ExecutorTool(name); executor {
					if name != codex.ToolName(tool) {
						return verdict.Failed(fmt.Sprintf("the hook saw %q, the harness spells %q", name, codex.ToolName(tool)), pointer)
					}
					return verdict.Passed("the hook saw "+name, pointer)
				}
				if strings.Contains(name, hostproto.ToolFetchRequest) || strings.Contains(name, hostproto.ToolSubmitResponse) {
					return verdict.Failed(fmt.Sprintf("the hook saw %q, the module spells %q", name, codex.ToolNames()), pointer)
				}
			}
			for _, item := range codexStream(found).ItemsOf(evidence.CodexItemMCPCall) {
				if item.Tool != hostproto.ToolFetchRequest && item.Tool != hostproto.ToolSubmitResponse {
					continue
				}
				pointer := evidence.StreamAt(item.Line, "mcp_tool_call")
				if item.Server != codex.Server() {
					return verdict.Failed(fmt.Sprintf("the harness runs the executor tool on server %q, the module names it %q", item.Server, codex.Server()), pointer)
				}
				return verdict.Undecided("the stream shows server "+item.Server+" but no hook payload recorded the tool name", pointer)
			}
			return verdict.Undecided("no executor tool call was recorded")
		})
}
