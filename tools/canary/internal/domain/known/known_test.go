package known_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/domain/known"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/verdict"
)

const (
	claudeVersionLine = "2.1.295 (Claude Code)"
	witnessSummary    = "the index of the raw API log is empty: classifier requests were not logged (cli_version_not_pinned)"
	entryJSON         = `{"harness":"claude","cli_version":"2.1.295","check":"classify_witness_recorded","reason":"cli_version_not_pinned","declared":"2026-10-11","reference":"ADR 0005, fact F: classifier requests not logged"}`
	committedRegistry = "../../../known-inconclusive.json"
)

func registry(t *testing.T) known.Registry {
	t.Helper()
	parsed, err := known.Parse([]byte(`{"entries":[` + entryJSON + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func report(verdicts ...verdict.Verdict) verdict.Report {
	return verdict.Report{Harness: "claude", Verdicts: verdicts}
}

func witness(status verdict.Status, summary string) verdict.Verdict {
	return verdict.Verdict{Check: "classify_witness_recorded", Status: status, Summary: summary}
}

func passing() verdict.Verdict { return verdict.Verdict{Check: "cli_version", Status: verdict.Pass} }

func TestAnnotateAndExitCode(t *testing.T) {
	t.Run("a matching inconclusive check stays inconclusive, carries the known line and lets the report exit with the success code [verifies SP-1527~1]", func(t *testing.T) {
		got := registry(t).Annotate(report(passing(), witness(verdict.Inconclusive, witnessSummary)), claudeVersionLine)
		if got.Verdicts[1].Status != verdict.Inconclusive {
			t.Errorf("status = %s", got.Verdicts[1].Status)
		}
		if want := "known: declared 2026-10-11, ADR 0005, fact F: classifier requests not logged"; got.Verdicts[1].Known != want {
			t.Errorf("known = %q, want %q", got.Verdicts[1].Known, want)
		}
		if got.Verdicts[0].Known != "" {
			t.Errorf("a passing check carries %q", got.Verdicts[0].Known)
		}
		if code := got.ExitCode(); code != verdict.ExitAllPass {
			t.Errorf("exit code %d", code)
		}
	})

	t.Run("a different version line, harness, check or reason does not match and keeps the inconclusive exit code [verifies SP-1527~1]", func(t *testing.T) {
		other := witness(verdict.Inconclusive, witnessSummary)
		for name, tc := range map[string]struct {
			report  verdict.Report
			version string
		}{
			"another version":     {report(passing(), other), "2.1.296 (Claude Code)"},
			"no version recorded": {report(passing(), other), ""},
			"another harness":     {verdict.Report{Harness: "copilot", Verdicts: []verdict.Verdict{other}}, claudeVersionLine},
			"another check":       {report(passing(), verdict.Verdict{Check: "other_check", Status: verdict.Inconclusive, Summary: witnessSummary}), claudeVersionLine},
			"another reason":      {report(passing(), witness(verdict.Inconclusive, "no classification was made")), claudeVersionLine},
		} {
			got := registry(t).Annotate(tc.report, tc.version)
			if code := got.ExitCode(); code != verdict.ExitInconclusive {
				t.Errorf("%s: exit code %d", name, code)
			}
		}
	})

	t.Run("a failure or an undeclared inconclusive check keeps the non-success exit code beside a declared one [verifies SP-1527~1]", func(t *testing.T) {
		declared := witness(verdict.Inconclusive, witnessSummary)
		failed := verdict.Verdict{Check: "hooks_fire", Status: verdict.Fail}
		undeclared := verdict.Verdict{Check: "stop_hook_fires", Status: verdict.Inconclusive, Summary: "no Stop hook"}
		if code := registry(t).Annotate(report(declared, failed), claudeVersionLine).ExitCode(); code != verdict.ExitSomeFailed {
			t.Errorf("with a failure: exit code %d", code)
		}
		if code := registry(t).Annotate(report(declared, undeclared), claudeVersionLine).ExitCode(); code != verdict.ExitInconclusive {
			t.Errorf("with an undeclared inconclusive: exit code %d", code)
		}
	})

	t.Run("an entry never covers a check that failed or passed [verifies SP-1527~1]", func(t *testing.T) {
		got := registry(t).Annotate(report(witness(verdict.Fail, witnessSummary), witness(verdict.Pass, witnessSummary)), claudeVersionLine)
		for _, v := range got.Verdicts {
			if v.Known != "" {
				t.Errorf("%s verdict carries %q", v.Status, v.Known)
			}
		}
	})

	t.Run("the empty registry marks nothing [verifies SP-1527~1]", func(t *testing.T) {
		got := known.Registry{}.Annotate(report(witness(verdict.Inconclusive, witnessSummary)), claudeVersionLine)
		if got.Verdicts[0].Known != "" || got.ExitCode() != verdict.ExitInconclusive {
			t.Errorf("verdicts = %+v", got.Verdicts)
		}
	})
}

func TestParse(t *testing.T) {
	t.Run("the committed registry is valid and declares the classifier witness of the pinned Claude Code [verifies SP-1527~1]", func(t *testing.T) {
		data, err := os.ReadFile(committedRegistry)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := known.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		got := parsed.Annotate(report(witness(verdict.Inconclusive, witnessSummary)), claudeVersionLine)
		if !strings.HasPrefix(got.Verdicts[0].Known, "known: declared 2026-10-11, ADR 0005") {
			t.Errorf("known = %q", got.Verdicts[0].Known)
		}
	})

	t.Run("a registry that breaks the schema is rejected [verifies SP-1527~1]", func(t *testing.T) {
		for name, body := range map[string]string{
			"not JSON":              `entries`,
			"no entries member":     `{}`,
			"unknown member":        `{"entries":[],"extra":1}`,
			"unknown entry member":  `{"entries":[` + strings.Replace(entryJSON, `"reason"`, `"why":"x","reason"`, 1) + `]}`,
			"missing reference":     `{"entries":[` + strings.Replace(entryJSON, `"reference":"ADR 0005, fact F: classifier requests not logged"`, `"reference":""`, 1) + `]}`,
			"missing harness":       `{"entries":[` + strings.Replace(entryJSON, `"harness":"claude",`, ``, 1) + `]}`,
			"bad date":              `{"entries":[` + strings.Replace(entryJSON, `2026-10-11`, `11/10/2026`, 1) + `]}`,
			"impossible date":       `{"entries":[` + strings.Replace(entryJSON, `2026-10-11`, `2026-13-40`, 1) + `]}`,
			"version with a suffix": `{"entries":[` + strings.Replace(entryJSON, `"2.1.295"`, `"2.1.295 (Claude Code)"`, 1) + `]}`,
			"version range":         `{"entries":[` + strings.Replace(entryJSON, `"2.1.295"`, `"2.1"`, 1) + `]}`,
			"trailing data":         `{"entries":[]} {}`,
		} {
			if _, err := known.Parse([]byte(body)); !errors.Is(err, known.ErrMalformed) {
				t.Errorf("%s: err = %v", name, err)
			}
		}
	})

	t.Run("an empty list of entries is a valid registry [verifies SP-1527~1]", func(t *testing.T) {
		if _, err := known.Parse([]byte(`{"entries":[]}`)); err != nil {
			t.Error(err)
		}
	})
}
