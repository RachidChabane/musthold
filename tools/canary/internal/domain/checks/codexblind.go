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
		"Every thread of the executor starts with none of the main agent's history: by the rule of the host protocol, its rollout holds only the harness context before the task message. "+
			"The report also says which spawns of the executor set a blind form (`fork_turns: \"none\"` or `fork_context: false`) and whether the module's pre-tool hook denied those that set none.",
		func(found evidence.Evidence) verdict.Verdict {
			agentType := codexStrategy().AgentType(found.Session.Role)
			children := found.Rollouts.ChildrenOf(agentType)
			notes := spawnGuardNote(found, agentType) + startPayloadNote(found)
			if len(children) == 0 {
				return verdict.Undecided(noChildNote(found, agentType)+notes, append(spawnPointers(found, agentType), startPointers(found)...)...)
			}
			if child, inherited := childWith(children, hostproto.ChildInherited); inherited {
				return verdict.Failed(fmt.Sprintf("thread %s of %s started with history of the main agent (%s)%s", child.ThreadID, agentType, child.HistoryReason, notes), childPointer(child))
			}
			if child, unproven := childWith(children, hostproto.ChildUnproven); unproven {
				return verdict.Undecided(fmt.Sprintf("the rollout of thread %s of %s cannot show the history it started with (%s)%s", child.ThreadID, agentType, child.HistoryReason, notes), childPointer(child))
			}
			pointers := make([]verdict.Pointer, 0, len(children))
			for _, child := range children {
				pointers = append(pointers, childPointer(child))
			}
			summary := fmt.Sprintf("%d thread(s) of %s started with none of the main agent's history%s", len(children), agentType, notes)
			return verdict.Passed(summary, append(pointers, startPointers(found)...)...)
		})
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

func spawnPointers(found evidence.Evidence, agentType string) []verdict.Pointer {
	var pointers []verdict.Pointer
	for _, spawn := range found.Rollouts.SpawnsOf(agentType) {
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
// many set a blind form, and of those that set none, how many the module's
// pre-tool hook denied.
func spawnGuardNote(found evidence.Evidence, agentType string) string {
	spawns := found.Rollouts.SpawnsOf(agentType)
	if len(spawns) == 0 {
		return ""
	}
	blind, denied, let := 0, 0, 0
	for _, spawn := range spawns {
		switch {
		case spawn.Blind:
			blind++
		case spawnDenied(found, spawn):
			denied++
		default:
			let++
		}
	}
	return fmt.Sprintf("; of %d spawn(s) of %s, %d set a blind form, %d that set none were denied by the pre-tool hook and %d that set none were not denied",
		len(spawns), agentType, blind, denied, let)
}

// spawnDenied reports whether the module's pre-tool hook denied the spawn: it
// answered the payload of the same tool use with a denial.
func spawnDenied(found evidence.Evidence, spawn evidence.RolloutSpawn) bool {
	for _, call := range found.HookCalls(primaryModule().Binary, hookKindPreTool) {
		payload := call.Payload()
		if payload.ToolUseID == spawn.CallID && hostproto.IsCodexSpawnTool(payload.ToolName) && call.Decision().Permission == string(hostproto.PermissionDeny) {
			return true
		}
	}
	return false
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
