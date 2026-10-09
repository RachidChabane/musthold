package checks_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

const classifyWitnessCheck = "classify_witness_recorded"

// withoutClassification is the recorded session before the mod classified anything.
func withoutClassification(t *testing.T) evidence.Files {
	t.Helper()
	files := readFiles(t, claudeSession)
	files.Journal = dropLines(files.Journal, "classify_")
	return files
}

// journalWith appends events to the journal of the recorded Claude Code session.
func journalWith(t *testing.T, files evidence.Files, events ...hostproto.Event) evidence.Files {
	t.Helper()
	var lines []string
	for _, event := range events {
		encoded, err := json.Marshal(hostproto.Record{Event: event})
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(encoded))
	}
	files.Journal = append(append([]byte{}, files.Journal...), []byte("\n"+strings.Join(lines, "\n")+"\n")...)
	return files
}

func shadowEvent() hostproto.Event {
	return hostproto.Event{Kind: hostproto.EventClassifyShadow, Ticket: "t.0.1", Detail: `{"source":"host-classify(5,rot1):m*5","cliVersion":"2.1.293"}`}
}

func observation(result hostproto.WitnessResult) hostproto.Event {
	return hostproto.Event{Kind: hostproto.EventClassifyShadowWitnessed, Ticket: "t.0.1", Detail: hostproto.ShadowWitnessDetail(result)}
}

func TestClassifyWitnessCheck(t *testing.T) {
	t.Run("a classification the raw API log holds as dictated passes and records the query source observed [verifies SP-1925~1]", func(t *testing.T) {
		files := journalWith(t, withoutClassification(t), shadowEvent(), observation(hostproto.WitnessResult{
			Verdict: hostproto.WitnessRaised, Model: "claude-sonnet-5-5-20260101", QuerySources: []string{"hook_prompt"}, LoggedCalls: 5,
		}))
		v := want(t, evaluate(t, "claude", files), classifyWitnessCheck, verdict.Pass)
		if !strings.Contains(v.Summary, "query source observed: hook_prompt") || len(v.Evidence) == 0 {
			t.Errorf("summary %q, evidence %v", v.Summary, v.Evidence)
		}
	})

	t.Run("a log that holds other calls than the dictated ones fails [verifies SP-1925~1]", func(t *testing.T) {
		for name, result := range map[string]hostproto.WitnessResult{
			"a contradicted label": {Verdict: hostproto.WitnessLabelsContradicted, LoggedCalls: 5},
			"a missing call":       {Verdict: hostproto.WitnessNone, Reason: hostproto.WitnessReasonMissing, LoggedCalls: 12},
			"an extra call":        {Verdict: hostproto.WitnessNone, Reason: hostproto.WitnessReasonExtra, LoggedCalls: 6},
		} {
			files := journalWith(t, withoutClassification(t), shadowEvent(), observation(result))
			if v := want(t, evaluate(t, "claude", files), classifyWitnessCheck, verdict.Fail); !strings.Contains(v.Summary, "does not hold") {
				t.Errorf("%s: summary %q", name, v.Summary)
			}
		}
	})

	t.Run("an empty index, no check or no classification is inconclusive and touches no other check [verifies SP-1925~1]", func(t *testing.T) {
		cases := map[string][]hostproto.Event{
			"no classification":   nil,
			"no check of the log": {shadowEvent()},
			"an empty index":      {shadowEvent(), observation(hostproto.WitnessResult{Verdict: hostproto.WitnessNone, Reason: hostproto.WitnessReasonMissing})},
		}
		baseline := evaluate(t, "claude", withoutClassification(t))
		for name, events := range cases {
			files := withoutClassification(t)
			if events != nil {
				files = journalWith(t, files, events...)
			}
			got := evaluate(t, "claude", files)
			want(t, got, classifyWitnessCheck, verdict.Inconclusive)
			for check, v := range baseline {
				if check != classifyWitnessCheck && got[check].Status != v.Status {
					t.Errorf("%s: %s changed from %s to %s", name, check, v.Status, got[check].Status)
				}
			}
		}
	})
}
