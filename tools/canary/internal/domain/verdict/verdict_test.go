package verdict_test

import (
	"encoding/json"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/domain/verdict"
)

func TestReportExitCode(t *testing.T) {
	report := func(statuses ...verdict.Status) verdict.Report {
		var verdicts []verdict.Verdict
		for _, status := range statuses {
			verdicts = append(verdicts, verdict.Verdict{Status: status})
		}
		return verdict.Report{Harness: "claude", Verdicts: verdicts}
	}

	t.Run("a report that only passes exits 0, an inconclusive one 3 and a failing one 1 [verifies SP-1501~1]", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			report verdict.Report
			want   int
		}{
			{"all pass", report(verdict.Pass, verdict.Pass), verdict.ExitAllPass},
			{"one inconclusive", report(verdict.Pass, verdict.Inconclusive), verdict.ExitInconclusive},
			{"one failed and one inconclusive", report(verdict.Inconclusive, verdict.Fail), verdict.ExitSomeFailed},
			{"nothing was concluded", report(), verdict.ExitInconclusive},
		} {
			if got := tc.report.ExitCode(); got != tc.want {
				t.Errorf("%s: exit code %d, want %d", tc.name, got, tc.want)
			}
		}
	})

	t.Run("a verdict is printed as JSON with an empty list of evidence rather than none [verifies SP-1501~1]", func(t *testing.T) {
		encoded, err := json.Marshal(verdict.Passed("ok").Named("check", "claim"))
		if err != nil {
			t.Fatal(err)
		}
		want := `{"check":"check","proves":"claim","status":"PASS","summary":"ok","evidence":[]}`
		if string(encoded) != want {
			t.Errorf("verdict = %s, want %s", encoded, want)
		}
	})

	t.Run("a pointer renders as file and line [verifies SP-1501~1]", func(t *testing.T) {
		if got := (verdict.Pointer{File: "stream.jsonl", Line: 12}).String(); got != "stream.jsonl:12" {
			t.Errorf("pointer = %q", got)
		}
		if got := (verdict.Pointer{File: "nocall.json"}).String(); got != "nocall.json" {
			t.Errorf("pointer = %q", got)
		}
	})
}
