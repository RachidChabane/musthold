package checks

import (
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

// forkTurnsNone is the value of fork_turns that starts a child with none of the
// parent's history.
const forkTurnsNone = "none"

// startPayloadMembersNaming are the fragments of a SubagentStart payload member
// that would tell a hook how much of the parent's history the child inherited.
var startPayloadMembersNaming = []string{"fork", "turn", "depth", "history"}

func executorSpawnBlindCheck() Check {
	return newCheck(checkCodexSpawnBlind,
		"The executor is spawned with `fork_turns` set to `\"none\"`: a spawn that omits it, or sets `\"all\"` or a number, hands the child the main agent's history, which breaks the executor's blindness.",
		func(found evidence.Evidence) verdict.Verdict {
			agentType := codexStrategy().AgentType(found.Session.Role)
			calls := found.Rollouts.SpawnsOf(agentType)
			hookNote := startPayloadNote(found)
			if len(calls) == 0 {
				return verdict.Undecided(noSpawnRecordNote(found, agentType)+hookNote, startPointers(found)...)
			}
			for _, call := range calls {
				pointer := evidence.At(evidence.RolloutMainFile, call.Line, evidence.RolloutSpawnTool)
				if !call.ForkTurnsSet {
					return verdict.Failed("the spawn of "+agentType+" omits fork_turns, so the child inherits the whole history of the main agent"+hookNote, pointer)
				}
				if call.ForkTurns != forkTurnsNone {
					return verdict.Failed(fmt.Sprintf("the spawn of %s sets fork_turns to %q, so the child inherits history of the main agent%s", agentType, call.ForkTurns, hookNote), pointer)
				}
			}
			pointers := []verdict.Pointer{evidence.At(evidence.RolloutMainFile, calls[0].Line, evidence.RolloutSpawnTool)}
			return verdict.Passed(fmt.Sprintf("%d spawn(s) of %s set fork_turns to %q%s", len(calls), agentType, forkTurnsNone, hookNote), append(pointers, startPointers(found)...)...)
		})
}

// noSpawnRecordNote says why no spawn of the executor could be read: the session
// started none, or it started one the stream shows but no rollout recorded.
func noSpawnRecordNote(found evidence.Evidence, agentType string) string {
	if spawns, _ := spawnedExecutor(found); len(spawns) > 0 {
		return "a spawn of " + agentType + " is in the stream, but no rollout recorded its arguments"
	}
	return checkCodexNoSpawnNote
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
