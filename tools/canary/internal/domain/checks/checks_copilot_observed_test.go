package checks_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/verdict"
)

const (
	observedCheckName = "run_observed_journaled"
	journalTicket     = "864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1"
	// launchBareFormat is a journal record of launch_bare for the session's ticket and the
	// agent named by the %s verb.
	launchBareFormat = `{"seq":98,"prev":"0","event":{"kind":"launch_bare","ticket":"` + journalTicket + `","agentId":"%s","role":"musthold-dp:decider"},"digest":"d"}` + "\n"
)

func TestCopilotRunObservedJournaled(t *testing.T) {
	launchBare := func(agentID string) string { return fmt.Sprintf(launchBareFormat, agentID) }
	cases := []struct {
		name   string
		status verdict.Status
		edit   func(*evidence.Files)
		says   string
	}{
		{"an allowlist the stream shows held and the product journaled for the executor with the same selection passes", verdict.Pass, func(*evidence.Files) {}, "stayed restricted-unverified"},
		{"an allowlist that held with no allowlist_observed in the journal fails", verdict.Fail, func(f *evidence.Files) {
			f.Journal = dropLines(f.Journal, `"allowlist_observed"`)
		}, "no allowlist_observed"},
		{"an allowlist_observed that records another selection than the stream shows fails", verdict.Fail, func(f *evidence.Files) {
			f.Journal = replace(f.Journal, `\"musthold-dp-exec/submit_response\"]`, `\"musthold-dp-exec/submit_response\",\"bash\"]`)
		}, "records the selection"},
		{"an allowlist_observed whose detail is not the document the product writes fails", verdict.Fail, func(f *evidence.Files) {
			f.Journal = replace(f.Journal, `"detail":"{\"cliVersion\"`, `"detail":"[{\"cliVersion\"`)
		}, "not the document"},
		{"an allowlist_observed journaled for another agent than the executor fails", verdict.Fail, func(f *evidence.Files) {
			f.Journal = replace(f.Journal, `"agentId":"`+executorAgentID+`"`, `"agentId":"another-agent"`)
		}, "no allowlist_observed for the executor"},
		{"an allowlist_observed for another ticket is no evidence for the session's ticket and fails", verdict.Fail, func(f *evidence.Files) {
			f.Journal = replace(f.Journal, `"kind":"allowlist_observed","ticket":"`+journalTicket, `"kind":"allowlist_observed","ticket":"88ec618dd2f5f5a73120577c8324765babf222a1fe52b144337a61bc9116d709.2.1`)
		}, "no allowlist_observed for the executor"},
		{"a launch_bare of the executor raises the answer and fails", verdict.Fail, func(f *evidence.Files) {
			f.Journal = append(f.Journal, []byte(launchBare(executorAgentID))...)
		}, "raised to restricted"},
		{"a launch_bare of another agent is a launch judged bare for the bait prompt and fails", verdict.Fail, func(f *evidence.Files) {
			f.Journal = append(f.Journal, []byte(launchBare("another-agent"))...)
		}, "launch_bare was journaled"},
		{"an allowlist_observed journaled while the allowlist check failed fails", verdict.Fail, func(f *evidence.Files) {
			f.Stream = replace(f.Stream, `"musthold-dp-exec/submit_response"]`, `"musthold-dp-exec/submit_response","bash"]`)
		}, "allowlist check failed"},
		{"no allowlist_observed for a run whose allowlist check failed passes", verdict.Pass, func(f *evidence.Files) {
			f.Stream = replace(f.Stream, `"musthold-dp-exec/submit_response"]`, `"musthold-dp-exec/submit_response","bash"]`)
			f.Journal = dropLines(f.Journal, `"allowlist_observed"`)
		}, "did not hold"},
		{"an inconclusive allowlist check leaves the journal with nothing to be compared with", verdict.Inconclusive, func(f *evidence.Files) {
			f.SandboxProbe = nil
		}, "inconclusive"},
		{"a stream with no executor subagent leaves the check inconclusive", verdict.Inconclusive, func(f *evidence.Files) {
			f.Stream = dropLines(f.Stream, `"subagent.`)
		}, ""},
		{"a stream with no subagent.selected leaves the check inconclusive", verdict.Inconclusive, func(f *evidence.Files) {
			f.Stream = dropLines(f.Stream, `"subagent.selected"`)
		}, "subagent.selected"},
	}
	for _, tc := range cases {
		t.Run(tc.name+" [verifies SP-1873~1]", func(t *testing.T) {
			files := readFiles(t, copilotSession)
			tc.edit(&files)
			v := want(t, evaluate(t, "copilot", files), observedCheckName, tc.status)
			if !strings.Contains(v.Summary, tc.says) {
				t.Errorf("summary = %s, want it to contain %q", v.Summary, tc.says)
			}
			if len(v.Evidence) == 0 && tc.status != verdict.Inconclusive {
				t.Errorf("%s: no evidence pointer", v.Summary)
			}
		})
	}

	t.Run("a session the harness refused leaves the check inconclusive [verifies SP-1873~1]", func(t *testing.T) {
		files := readFiles(t, copilotSession)
		files.Stream = append(files.Stream, []byte(`{"id":"e1","type":"session.error","data":{"errorType":"quota","message":"no credits"}}`+"\n")...)
		want(t, evaluate(t, "copilot", files), observedCheckName, verdict.Inconclusive)
	})

	t.Run("the verdict of the recorded session points at the journal entry and the selection [verifies SP-1873~1]", func(t *testing.T) {
		v := want(t, evaluate(t, "copilot", readFiles(t, copilotSession)), observedCheckName, verdict.Pass)
		if len(v.Evidence) != 2 || v.Evidence[0].File != "journal.jsonl" || v.Evidence[1].File != "stream.jsonl" {
			t.Errorf("evidence = %+v", v.Evidence)
		}
	})
}
