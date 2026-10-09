package checks

import (
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

// startPayloadMembersNaming are the fragments of a SubagentStart payload member
// that would tell a hook how much of the parent's history the child inherited.
var startPayloadMembersNaming = []string{"fork", "turn", "depth", "history"}

// sessionMetaNote points at the first line of a child's rollout.
const sessionMetaNote = "session_meta"

func executorSpawnBlindCheck() Check {
	return newCheck(checkCodexSpawnBlind,
		"The scripted session first spawns the executor with no history control: the module's pre-tool hook must deny that spawn, and the spawn the denial asks for must start a thread that, by the rule of the host protocol, "+
			"holds only the harness context before its task message. A spawn without a blind form (`fork_turns: \"none\"` or `fork_context: false`) that starts a thread fails.",
		func(found evidence.Evidence) verdict.Verdict {
			agentType := codexStrategy().AgentType(found.Session.Role)
			spawns := found.Rollouts.SpawnsOf(agentType)
			children := found.Rollouts.ChildrenOf(agentType)
			notes := spawnGuardNote(found, agentType) + threadsNote(children, agentType) + startPayloadNote(found)
			if len(spawns) == 0 && len(children) == 0 {
				return verdict.Undecided(noChildNote(found, agentType)+notes, startPointers(found)...)
			}
			if spawn, child, reached := unblindSpawnThatStarted(found, spawns); reached {
				return verdict.Failed(fmt.Sprintf("the spawn of %s at line %d sets no blind form and started thread %s%s", agentType, spawn.Line, child, notes),
					evidence.At(evidence.RolloutMainFile, spawn.Line, evidence.RolloutSpawnTool))
			}
			if child, inherited := childWith(children, hostproto.ChildInherited); inherited {
				return verdict.Failed(fmt.Sprintf("thread %s of %s started with history of the main agent (%s)%s", child.ThreadID, agentType, child.HistoryReason, notes), threadPointers(found, child)...)
			}
			if len(spawns) == 0 {
				return verdict.Undecided("a thread of "+agentType+" is recorded, but no rollout recorded the spawns that started it"+notes, childPointers(children)...)
			}
			first := spawns[0]
			firstPointer := evidence.At(evidence.RolloutMainFile, first.Line, evidence.RolloutSpawnTool)
			denial, seen := spawnHookCall(found, first)
			switch {
			case first.Blind:
				return verdict.Undecided("the first spawn of "+agentType+" already set a blind form, so the refusal of the pre-tool hook was not exercised"+notes, firstPointer)
			case !seen || denial.Decision().Permission != string(hostproto.PermissionDeny):
				return verdict.Undecided("the first spawn of "+agentType+" set no blind form, and no denial of it by the module's pre-tool hook is recorded"+notes, firstPointer)
			case len(children) == 0:
				return verdict.Undecided("the pre-tool hook denied the first spawn of "+agentType+", but no rollout of a thread started after it was recorded"+notes, firstPointer)
			}
			if child, unproven := childWith(children, hostproto.ChildUnproven); unproven {
				return verdict.Undecided(fmt.Sprintf("the rollout of thread %s of %s cannot show the history it started with (%s)%s", child.ThreadID, agentType, child.HistoryReason, notes), childPointer(child))
			}
			pointers := append([]verdict.Pointer{evidence.HooksAt(denial.Line, "pre-tool denial of the first spawn"), firstPointer}, childPointers(children)...)
			summary := fmt.Sprintf("the module's pre-tool hook denied the first spawn of %s, which set no blind form, and the spawn that followed started blind%s", agentType, notes)
			return verdict.Passed(summary, pointers...)
		})
}

// unblindSpawnThatStarted returns a spawn that set no blind form and yet started
// a thread, with the thread its output names.
func unblindSpawnThatStarted(found evidence.Evidence, spawns []evidence.RolloutSpawn) (evidence.RolloutSpawn, string, bool) {
	for _, spawn := range spawns {
		if spawn.Blind {
			continue
		}
		if spawn.ChildThreadID != "" {
			return spawn, spawn.ChildThreadID, true
		}
		if spawn.ChildPath != "" {
			return spawn, spawn.ChildPath, true
		}
	}
	return evidence.RolloutSpawn{}, "", false
}

// threadsNote reports how the recorded threads of the executor started.
func threadsNote(children []evidence.RolloutChild, agentType string) string {
	if len(children) == 0 {
		return ""
	}
	counts := map[hostproto.ChildHistory]int{}
	for _, child := range children {
		counts[child.History]++
	}
	return fmt.Sprintf("; of %d thread(s) of %s, %d started blind, %d inherited history and %d cannot show it",
		len(children), agentType, counts[hostproto.ChildBlind], counts[hostproto.ChildInherited], counts[hostproto.ChildUnproven])
}

func childPointers(children []evidence.RolloutChild) []verdict.Pointer {
	pointers := make([]verdict.Pointer, 0, len(children))
	for _, child := range children {
		pointers = append(pointers, childPointer(child))
	}
	return pointers
}

func childWith(children []evidence.RolloutChild, history hostproto.ChildHistory) (evidence.RolloutChild, bool) {
	for _, child := range children {
		if child.History == history {
			return child, true
		}
	}
	return evidence.RolloutChild{}, false
}

func childPointer(child evidence.RolloutChild) verdict.Pointer {
	return evidence.At(evidence.RolloutChildrenDir+"/"+child.File, 1, sessionMetaNote)
}

// threadPointers point at the rollout of a thread and at the spawn call that
// started it, when the main rollout records one.
func threadPointers(found evidence.Evidence, child evidence.RolloutChild) []verdict.Pointer {
	pointers := []verdict.Pointer{childPointer(child)}
	if spawn, recorded := found.Rollouts.SpawnOf(child); recorded {
		pointers = append(pointers, evidence.At(evidence.RolloutMainFile, spawn.Line, evidence.RolloutSpawnTool))
	}
	return pointers
}

// noChildNote says why no thread of the executor could be judged: the session
// started none, or no rollout of the thread it started was recorded.
func noChildNote(found evidence.Evidence, agentType string) string {
	if spawns, _ := spawnedExecutor(found); len(spawns) > 0 {
		return "a start of " + agentType + " is recorded, but no rollout of its thread was recorded"
	}
	return checkCodexNoSpawnNote
}

// spawnGuardNote reports the spawns of the executor the main rollout records: how
// many the module's pre-tool hook received, how many set a blind form, and of
// those that set none, how many the hook denied.
func spawnGuardNote(found evidence.Evidence, agentType string) string {
	spawns := found.Rollouts.SpawnsOf(agentType)
	if len(spawns) == 0 {
		return ""
	}
	received, blind, denied, let := 0, 0, 0, 0
	for _, spawn := range spawns {
		if _, seen := spawnHookCall(found, spawn); seen {
			received++
		}
		switch {
		case spawn.Blind:
			blind++
		case spawnDenied(found, spawn):
			denied++
		default:
			let++
		}
	}
	return fmt.Sprintf("; of %d spawn(s) of %s, the module's pre-tool hook received %d, %d set a blind form, %d that set none were denied by the pre-tool hook and %d that set none were not denied",
		len(spawns), agentType, received, blind, denied, let)
}

// spawnHookCall is the module's pre-tool hook call on the same tool use as the
// spawn.
func spawnHookCall(found evidence.Evidence, spawn evidence.RolloutSpawn) (evidence.HookCall, bool) {
	for _, call := range found.HookCalls(primaryModule().Binary, hookKindPreTool) {
		payload := call.Payload()
		if payload.ToolUseID == spawn.CallID && hostproto.IsCodexSpawnTool(payload.ToolName) {
			return call, true
		}
	}
	return evidence.HookCall{}, false
}

// spawnDenied reports whether the module's pre-tool hook answered the spawn with
// a denial.
func spawnDenied(found evidence.Evidence, spawn evidence.RolloutSpawn) bool {
	call, seen := spawnHookCall(found, spawn)
	return seen && call.Decision().Permission == string(hostproto.PermissionDeny)
}

func startPointers(found evidence.Evidence) []verdict.Pointer {
	calls := found.HookCalls(primaryModule().Binary, hookKindStart)
	if len(calls) == 0 {
		return nil
	}
	return []verdict.Pointer{evidence.HooksAt(calls[0].Line, "subagent-start payload")}
}

// startPayloadNote reports what a SubagentStart hook receives: the members of its
// payload, and whether any of them says how much history the child inherited.
func startPayloadNote(found evidence.Evidence) string {
	calls := found.HookCalls(primaryModule().Binary, hookKindStart)
	if len(calls) == 0 {
		return "; no SubagentStart payload was recorded"
	}
	keys := calls[0].PayloadKeys()
	visible := "none of them says how much history the child inherited"
	for _, key := range keys {
		for _, fragment := range startPayloadMembersNaming {
			if strings.Contains(strings.ToLower(key), fragment) {
				visible = "member " + key + " may say how much history the child inherited"
			}
		}
	}
	return fmt.Sprintf("; the SubagentStart payload carries %s, and %s", strings.Join(keys, ", "), visible)
}
