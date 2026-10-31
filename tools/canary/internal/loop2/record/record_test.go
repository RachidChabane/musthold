package record

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	demorecord "github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	moduleDP        = "musthold-dp"
	verdictFail     = "fail"
	verdictPass     = "pass"
	exitFail        = 1
	criterionBroken = "FARE-7"
)

func gate(findings ...demorecord.GateFinding) *facts.Report {
	report := &facts.Report{}
	report.Verdict, report.ExitCode, report.Findings = verdictFail, exitFail, findings
	return report
}

func passingGate() *facts.Report {
	report := &facts.Report{}
	report.Verdict = verdictPass
	return report
}

func blocking(code, criterion, module string) demorecord.GateFinding {
	return demorecord.GateFinding{Category: code, Blocking: true, RequirementID: criterion, Module: module}
}

func upheld() *Adjudication {
	return &Adjudication{Criterion: criterionBroken, Sentence: "A rider under 12 travels free on weekends.", Judges: []Verdict{
		{Judge: JudgeTerra, Defective: true}, {Judge: JudgeOpus, Defective: true},
	}}
}

func input(outcome study.Outcome, final *facts.Report, defective bool) facts.Input {
	in := facts.Input{Stops: map[int]*facts.Report{}, FinalGate: final}
	in.Run.Task, in.Run.Arm, in.Run.Repetition, in.Run.Outcome = "fare", "treatment", 1, outcome
	in.Oracle = study.Oracle{Defective: defective}
	if defective {
		in.Oracle.Violated = []string{criterionBroken}
	}
	return in
}

func TestRefusalOf(t *testing.T) {
	t.Run("the refusal cause follows acceptance, the cap and loop-1's kind of the blocking findings [verifies SP-2801~1]", func(t *testing.T) {
		cases := []struct {
			name     string
			accepted bool
			outcome  study.Outcome
			gate     *facts.Report
			want     Cause
		}{
			{"accepted", true, study.OutcomeCompleted, passingGate(), CauseNone},
			{"accepted at a cap", true, study.OutcomeCappedTime, passingGate(), CauseNone},
			{"a cap before a verdict", false, study.OutcomeCappedMessages, gate(blocking("surviving_valid_mutant", criterionBroken, moduleDP)), CauseCap},
			{"a proof verdict", false, study.OutcomeCompleted, gate(blocking("surviving_valid_mutant", criterionBroken, moduleDP), blocking("stale_proof", "FARE-2", moduleDP)), CauseFinding},
			{"a stale proof alone", false, study.OutcomeCompleted, gate(blocking("stale_proof", criterionBroken, moduleDP)), CausePending},
			{"a pending reason alone", false, study.OutcomeCompleted, gate(blocking("awaiting_host", criterionBroken, moduleDP)), CausePending},
			{"an uncited requirement", false, study.OutcomeCompleted, gate(blocking("uncovered_requirement", criterionBroken, "")), CauseTraceability},
			{"no report", false, study.OutcomeCompleted, nil, CauseTraceability},
		}
		for _, c := range cases {
			got := RefusalOf(c.accepted, c.outcome, c.gate)
			if got.Cause != c.want || got.Findings == nil {
				t.Errorf("%s: %+v, want %s", c.name, got, c.want)
			}
		}
		got := RefusalOf(false, study.OutcomeCompleted, gate(blocking("surviving_valid_mutant", criterionBroken, moduleDP)))
		if len(got.Findings) != 1 || got.Findings[0] != (facts.Finding{Code: "surviving_valid_mutant", Criterion: criterionBroken, Module: moduleDP}) {
			t.Fatalf("findings %+v", got.Findings)
		}
	})
}

func TestAdjudication(t *testing.T) {
	t.Run("an adjudication holds one verdict of each judge, and is disputed when one does not hold the escape defective [verifies SP-2818~1]", func(t *testing.T) {
		if err := upheld().Validate(); err != nil || upheld().Disputed() {
			t.Fatalf("upheld: %v, disputed %v", err, upheld().Disputed())
		}
		disputed := upheld()
		disputed.Judges[1].Defective = false
		if !disputed.Disputed() {
			t.Fatal("a dissenting judge is a dispute")
		}
		for _, judges := range [][]Verdict{
			{{Judge: JudgeTerra, Defective: true}},
			{{Judge: JudgeTerra, Defective: true}, {Judge: JudgeTerra, Defective: true}},
			{{Judge: JudgeTerra, Defective: true}, {Judge: JudgeOpus, Defective: true}, {Judge: "claude-sonnet-5-5", Defective: true}},
		} {
			if err := (Adjudication{Judges: judges}).Validate(); !errors.Is(err, ErrAdjudication) {
				t.Errorf("%+v: %v", judges, err)
			}
		}
	})
}

func TestCompose(t *testing.T) {
	t.Run("a record carries loop-1's facts, the first-stop grade when there is one and the refusal cause [verifies SP-2800~1]", func(t *testing.T) {
		firstStop := &study.Oracle{Defective: true, Violated: []string{criterionBroken}}
		run, err := Compose("fare-treatment-1", input(study.OutcomeCompleted, passingGate(), false), firstStop, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !run.Accepted || run.Defective || run.Escaped || run.Task != "fare" || run.Refusal.Cause != CauseNone {
			t.Fatalf("facts %+v", run)
		}
		if run.FirstStop == nil || !run.FirstStop.Defective || run.FirstStop.Violated[0] != criterionBroken {
			t.Fatalf("first stop %+v", run.FirstStop)
		}
		none, err := Compose("fare-treatment-1", input(study.OutcomeCompleted, passingGate(), false), nil, nil)
		if err != nil || none.FirstStop != nil {
			t.Fatalf("no stop: %+v, %v", none.FirstStop, err)
		}
	})
	t.Run("an escape needs a valid adjudication [verifies SP-2800~1]", func(t *testing.T) {
		escape := input(study.OutcomeCompleted, passingGate(), true)
		if _, err := Compose("fare-treatment-1", escape, nil, nil); !errors.Is(err, ErrUnadjudicated) {
			t.Fatalf("no adjudication: %v", err)
		}
		if _, err := Compose("fare-treatment-1", escape, nil, &Adjudication{}); !errors.Is(err, ErrAdjudication) {
			t.Fatalf("no verdict: %v", err)
		}
		run, err := Compose("fare-treatment-1", escape, nil, upheld())
		if err != nil || run.Adjudication == nil || !run.Escaped {
			t.Fatalf("%+v, %v", run, err)
		}
		refused, err := Compose("fare-treatment-1", input(study.OutcomeCompleted, gate(blocking("surviving_valid_mutant", criterionBroken, moduleDP)), true), nil, nil)
		if err != nil || refused.Escaped || refused.Refusal.Cause != CauseFinding {
			t.Fatalf("a refused defective change needs no adjudication: %+v, %v", refused, err)
		}
	})
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	records := t.TempDir()
	meta := study.Run{Attempt: 1, Outcome: study.OutcomeCompleted}
	meta.Task, meta.Arm, meta.Repetition = "fare", "treatment", 2
	writeJSON(t, filepath.Join(records, demorecord.MetaFile), meta)
	writeJSON(t, filepath.Join(records, demorecord.FinalGateFile), passingGate())
	writeJSON(t, filepath.Join(records, study.OracleFile), study.Oracle{Defective: true, Violated: []string{criterionBroken}})
	writeJSON(t, filepath.Join(records, study.HeldOutFile), study.HeldOut{})
	writeJSON(t, filepath.Join(records, study.DetectorsFile), study.Detection{})
	writeJSON(t, filepath.Join(records, FirstStopOracleFile), study.Oracle{Defective: true, Violated: []string{criterionBroken}})

	t.Run("an escape's records without adjudication.json are refused [verifies SP-2800~1]", func(t *testing.T) {
		if _, _, err := Load("fare-treatment-2", records, "<run>/project"); !errors.Is(err, ErrUnadjudicated) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("the records give the run with the digest of every file read [verifies SP-2800~1]", func(t *testing.T) {
		writeJSON(t, filepath.Join(records, AdjudicationFile), upheld())
		run, digests, err := Load("fare-treatment-2", records, "<run>/project")
		if err != nil {
			t.Fatal(err)
		}
		if !run.Escaped || run.Repetition != 2 || run.FirstStop == nil || run.Adjudication == nil {
			t.Fatalf("run %+v", run)
		}
		for _, file := range []string{demorecord.MetaFile, study.OracleFile, FirstStopOracleFile, AdjudicationFile} {
			if len(digests[file]) != 64 {
				t.Errorf("no digest of %s: %v", file, digests)
			}
		}
	})
	t.Run("a malformed added file is refused [verifies SP-2800~1]", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(records, FirstStopOracleFile), []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Load("fare-treatment-2", records, "<run>/project"); err == nil {
			t.Fatal("a malformed first-stop grade was read")
		}
	})
}
