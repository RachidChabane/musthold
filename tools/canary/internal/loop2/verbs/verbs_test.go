package verbs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	demorecord "github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	l1analyze "github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/stamp"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/analysis"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
)

const (
	broken   = "T-1"
	runsEach = 2
	protocol = "- 2026-11-03T08:00:00Z | protocol | all | §6 | the runs follow the order | one run started late"
)

// noTokens has no freeze token and no deviation token.
type noTokens struct{}

func (noTokens) Read(context.Context, string) (stamp.Token, error) {
	return stamp.Token{}, os.ErrNotExist
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, data)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// world is a repository with an inputs freeze and a work directory whose order
// holds two tasks of two runs per arm; the control escapes once per task.
type world struct {
	repo, inputs, work, results string
	order                       []string
	start                       time.Time
}

func newWorld(t *testing.T) world {
	t.Helper()
	w := world{repo: t.TempDir(), work: t.TempDir(), start: time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)}
	w.inputs = filepath.Join(w.repo, analyze.DefaultInputs)
	w.results = filepath.Join(w.repo, analyze.DefaultResults)
	for _, task := range []string{"fare", "toll"} {
		for _, arm := range l1analyze.Arms() {
			for repetition := 1; repetition <= runsEach; repetition++ {
				w.order = append(w.order, fmt.Sprintf("%s-%s-%d", task, arm, repetition))
			}
		}
	}
	writeJSON(t, w.inputs, study.Inputs{Commits: study.Commits{Prereg: "abc", Inputs: "def"}, Tasks: []study.TaskInputs{{Name: "fare"}, {Name: "toll"}}, Order: w.order})
	return w
}

// count writes the counted attempt of a run of the order.
func (w world) count(t *testing.T, name string) {
	t.Helper()
	parts := strings.Split(name, "-")
	escape := parts[1] == l1analyze.ArmControl && parts[2] == "1"
	records := study.Layout{Root: w.work}.Records(name, 1)
	meta := study.Run{Attempt: 1, Outcome: study.OutcomeCompleted}
	meta.Task, meta.Arm, meta.StartedAt = parts[0], parts[1], w.start
	fmt.Sscan(parts[2], &meta.Repetition)
	writeJSON(t, filepath.Join(records, demorecord.MetaFile), meta)
	gate := facts.Report{}
	gate.Verdict = "pass"
	writeJSON(t, filepath.Join(records, demorecord.FinalGateFile), gate)
	oracle := study.Oracle{Violated: []string{}}
	if escape {
		oracle = study.Oracle{Defective: true, Violated: []string{broken}}
		writeJSON(t, filepath.Join(records, record.AdjudicationFile), record.Adjudication{Criterion: broken, Sentence: "The rule.", Judges: []record.Verdict{
			{Judge: record.JudgeTerra, Defective: true}, {Judge: record.JudgeOpus, Defective: true},
		}})
	}
	writeJSON(t, filepath.Join(records, study.OracleFile), oracle)
	writeJSON(t, filepath.Join(records, record.FirstStopOracleFile), oracle)
	writeJSON(t, filepath.Join(records, study.HeldOutFile), study.HeldOut{})
	writeJSON(t, filepath.Join(records, study.DetectorsFile), study.Detection{})
}

func (w world) analyze(t *testing.T, now time.Time) error {
	t.Helper()
	return analyze.Analyze(context.Background(), analyze.Options{
		Repo: w.repo, Inputs: w.inputs, Work: w.work, Results: w.results, Now: now, Imprints: noTokens{}, Loader: record.Files{},
	}, io.Discard)
}

func TestTable(t *testing.T) {
	t.Run("the composition root names analyze, calibrate and report-lint [verifies SP-2828~1]", func(t *testing.T) {
		if !slices.Equal(Names(), []string{Analyze, Calibrate, ReportLint}) {
			t.Fatalf("names %v", Names())
		}
		if _, found := Lookup("run"); found {
			t.Fatal("a verb this root does not wire")
		}
	})
}

func TestAnalyze(t *testing.T) {
	w := newWorld(t)
	for _, name := range w.order[:len(w.order)-1] {
		w.count(t, name)
	}
	t.Run("before the run phase ends analyze refuses and writes nothing [verifies SP-2828~1]", func(t *testing.T) {
		if err := w.analyze(t, w.start.Add(time.Hour)); !errors.Is(err, phase.ErrOpen) {
			t.Fatalf("got %v", err)
		}
		if _, err := os.Stat(w.results); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("results written")
		}
	})
	w.count(t, w.order[len(w.order)-1])
	t.Run("a protocol deviation without its timestamp is refused [verifies SP-2828~1]", func(t *testing.T) {
		layout := study.Layout{Root: w.work}
		writeFile(t, layout.Deviations(), []byte(study.DeviationsTitle+"\n"+protocol+"\n"))
		if err := w.analyze(t, w.start.Add(time.Hour)); err == nil {
			t.Fatal("an untimestamped protocol deviation passed")
		}
		if err := os.Remove(layout.Deviations()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("analyze writes summary.json with every run, its record and the digests, and REPORT.md that lints clean [verifies SP-2825~1] [verifies SP-2827~1]", func(t *testing.T) {
		if err := w.analyze(t, w.start.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(w.results, l1analyze.SummaryFile))
		if err != nil {
			t.Fatal(err)
		}
		var summary analysis.Summary
		if err := json.Unmarshal(data, &summary); err != nil {
			t.Fatal(err)
		}
		if len(summary.Runs) != len(w.order) || summary.Runs[0].Facts == nil || summary.Runs[0].FirstStop == nil {
			t.Fatalf("runs %+v", summary.Runs)
		}
		if summary.Analysis.Primary.Control.Fraction != (facts.Fraction{K: 2, N: 4}) || len(summary.Analysis.Readjudication.Escapes) != 2 {
			t.Fatalf("analysis %+v", summary.Analysis.Primary)
		}
		if len(summary.Digests[study.InputsFile]) != 64 || len(summary.Digests["work/runs/fare-control-1-1/records/"+record.AdjudicationFile]) != 64 {
			t.Fatalf("digests %v", summary.Digests)
		}
		if summary.Provenance.Tags[1].Name != analysis.TagPrereg || summary.Provenance.Tags[1].Commit != "abc" {
			t.Fatalf("provenance %+v", summary.Provenance)
		}
		var stdout, stderr bytes.Buffer
		if code := Table()[ReportLint](context.Background(), []string{"--repo", w.repo}, &stdout, &stderr); code != 0 {
			t.Fatalf("report-lint exit %d: %s", code, stderr.String())
		}
	})
	t.Run("report-lint fails a tampered report [verifies SP-2827~1]", func(t *testing.T) {
		path := filepath.Join(w.results, l1analyze.ReportFile)
		data, _ := os.ReadFile(path)
		writeFile(t, path, append(data, []byte("\nThe gate works.\n")...))
		if code := Table()[ReportLint](context.Background(), []string{"--repo", w.repo}, io.Discard, io.Discard); code != 1 {
			t.Fatalf("exit %d", code)
		}
	})
	t.Run("the analyze verb's usage errors exit 2 [verifies SP-2828~1]", func(t *testing.T) {
		if code := Table()[Analyze](context.Background(), []string{"--repo", w.repo}, io.Discard, io.Discard); code != 2 {
			t.Fatalf("exit %d", code)
		}
	})
}
