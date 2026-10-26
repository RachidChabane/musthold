package checks

import (
	"fmt"
	"slices"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

// observedRun is what the product journaled about the executor of a session and
// what the stream shows of the same executor.
type observedRun struct {
	found    evidence.Evidence
	executor evidence.CopilotSubagent
	honoured verdict.Verdict
	events   []hostproto.Event
}

// journaledFor returns the journal entries of a kind for the session's ticket.
func (r observedRun) journaledFor(kind hostproto.EventKind) []evidence.JournalEntry {
	var entries []evidence.JournalEntry
	for _, entry := range r.found.Events(kind) {
		if entry.Event.Ticket == r.found.Session.Ticket {
			entries = append(entries, entry)
		}
	}
	return entries
}

// ofExecutor keeps the entries that name the executor subagent of the stream.
func (r observedRun) ofExecutor(entries []evidence.JournalEntry) []evidence.JournalEntry {
	return slices.DeleteFunc(slices.Clone(entries), func(entry evidence.JournalEntry) bool { return entry.Event.AgentID != r.executor.AgentID })
}

func journalPointers(entries []evidence.JournalEntry) []verdict.Pointer {
	pointers := make([]verdict.Pointer, len(entries))
	for index, entry := range entries {
		pointers[index] = evidence.JournalAt(entry.Line, string(entry.Event.Kind))
	}
	return pointers
}

// runObservedJournaledCheck judges the product against the same session: the
// product's stop hook reads the session log of the executor and journals what it
// observed, and the canary's bait prompt is not the bare ticket, so the product
// may observe the allowlist of the run and must not raise its answer.
func runObservedJournaledCheck() Check {
	return newCheck(checkCopilotRunObserved,
		"The product's stop hook observes the run it is given: it journals allowlist_observed for the executor with the selection the stream shows when the allowlist held, journals no allowlist_observed when it did not, journals no launch_bare for the canary's non-canonical bait prompt, and leaves the answer restricted-unverified.",
		func(found evidence.Evidence) verdict.Verdict {
			executor, started := executorSubagent(copilotStream(found), found.Session.Role)
			if !started {
				return verdict.Undecided(noExecutorAgent)
			}
			if !executor.HasSelected || executor.AgentID == "" {
				return verdict.Undecided("the stream holds no subagent.selected that carries the agent id of the executor, so the journal cannot be compared with it")
			}
			run := observedRun{found: found, executor: executor, honoured: allowlistCheck().Evaluate(found)}
			for _, entry := range found.Journal {
				if entry.Event.Ticket == found.Session.Ticket {
					run.events = append(run.events, entry.Event)
				}
			}
			return run.judge()
		})
}

func (r observedRun) judge() verdict.Verdict {
	observed := r.journaledFor(hostproto.EventAllowlistObserved)
	switch r.honoured.Status {
	case verdict.Inconclusive:
		return verdict.Undecided("the allowlist check is inconclusive, so the journal has nothing to be compared with: " + r.honoured.Summary)
	case verdict.Fail:
		if len(observed) > 0 {
			return verdict.Failed("allowlist_observed was journaled for a run whose allowlist check failed: "+r.honoured.Summary, journalPointers(observed)...)
		}
		return verdict.Passed("the product journaled no allowlist_observed for a run whose allowlist did not hold", r.honoured.Evidence...)
	}
	return r.judgeHonouredRun(observed)
}

// judgeHonouredRun judges a run whose allowlist the stream shows to have held.
func (r observedRun) judgeHonouredRun(observed []evidence.JournalEntry) verdict.Verdict {
	selected := evidence.StreamAt(r.executor.Selected.Line, r.executor.Selected.Type)
	mine := r.ofExecutor(observed)
	if len(mine) == 0 {
		return verdict.Failed("the allowlist held but the product journaled no allowlist_observed for the executor "+r.executor.AgentID, append(journalPointers(observed), selected)...)
	}
	if detail, mismatch := r.selectionMismatch(mine[0]); mismatch {
		return verdict.Failed(detail, evidence.JournalAt(mine[0].Line, string(mine[0].Event.Kind)), selected)
	}
	bare := r.journaledFor(hostproto.EventLaunchBare)
	if level := executorrun.Effective(unverifiedProfile(), r.events).Isolation; level != format.IsolationRestrictedUnverified {
		return verdict.Failed(fmt.Sprintf("the answer was raised to %s although the canary's prompt is not the bare ticket", level), append(journalPointers(mine), journalPointers(bare)...)...)
	}
	if len(bare) > 0 {
		return verdict.Failed("launch_bare was journaled for the canary's padded prompt, which is not the bare ticket", append(journalPointers(bare), selected)...)
	}
	return verdict.Passed(fmt.Sprintf("the product journaled allowlist_observed for the executor with the selection the stream shows (%v), no launch_bare for the padded prompt, and the answer stayed %s",
		r.executor.Selected.Tools, format.IsolationRestrictedUnverified),
		append(journalPointers(mine), selected)...)
}

// selectionMismatch says whether the selection an allowlist_observed event records
// differs from the one the stream shows for the executor.
func (r observedRun) selectionMismatch(entry evidence.JournalEntry) (string, bool) {
	recorded, decoded := executorrun.ParseAllowlistObserved(entry.Event.Detail)
	if !decoded {
		return "the detail of allowlist_observed is not the document the product writes: " + entry.Event.Detail, true
	}
	streamed := slices.Sorted(slices.Values(r.executor.Selected.Tools))
	if !slices.Equal(slices.Sorted(slices.Values(recorded.SelectedTools)), streamed) {
		return fmt.Sprintf("allowlist_observed records the selection %v, the stream shows %v", recorded.SelectedTools, r.executor.Selected.Tools), true
	}
	return "", false
}

// unverifiedProfile is the profile the answer of a Copilot executor is stored with.
func unverifiedProfile() format.ExecutorProfile {
	return format.ExecutorProfile{
		Channel:   format.ChannelHostSubagent,
		Harness:   format.Harness{Name: hostproto.HarnessCopilot},
		Isolation: format.IsolationRestrictedUnverified,
	}
}
