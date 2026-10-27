package analyze

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/stamp"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const recording = "../../demoloop/app/testdata/recording"

// fakeImprints answers every token with the SHA-256 of its text file, or with
// the time of a freeze token.
type fakeImprints struct{ wrong bool }

func (f fakeImprints) Read(_ context.Context, tsr string) (stamp.Token, error) {
	text, err := os.ReadFile(strings.TrimSuffix(tsr, stamp.TokenExtension) + stamp.TextExtension)
	if err != nil {
		return stamp.Token{}, err
	}
	digest := sha256.Sum256(text)
	if f.wrong {
		digest = sha256.Sum256(append(text, ' '))
	}
	return stamp.Token{Algorithm: stamp.SHA256, Imprint: hex.EncodeToString(digest[:]), Time: time.Date(2026, 10, 26, 15, 55, 27, 0, time.UTC)}, nil
}

type fixture struct {
	repo, work, home string
	options          Options
}

const (
	countedRun = "boxoffice-treatment-1"
	blockedRun = "boxoffice-control-1"
	protocol   = "- 2026-11-03T08:00:00Z | protocol | all | §6.5 | the batch resumes at any time | the operator resumed it at 08:00"
)

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := writeFile(path, data); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, data)
}

// newFixture lays out a repository and a work directory whose run phase has
// ended: one run counted on its second attempt, after a quota stop, and one run
// blocked after three harness errors.
func newFixture(t *testing.T) fixture {
	t.Helper()
	f := fixture{repo: t.TempDir(), home: t.TempDir()}
	f.work = filepath.Join(f.home, ".work")
	experiment := filepath.Join(f.repo, "experiments", "loop-1")
	mustWrite(t, filepath.Join(experiment, "loop-1.json"), []byte("{}"))
	mustJSON(t, filepath.Join(experiment, study.FreezeDirName, study.InputsFile), map[string]any{
		"order":        []string{blockedRun, countedRun},
		"commits":      map[string]string{"prereg": "9058fee", "inputs": "abc123"},
		"tasks":        []map[string]string{{"name": "boxoffice"}},
		"construction": records.Log{Drops: []records.Drop{{Domain: "water bill", Reason: "A5 failed"}}},
	})
	mustWrite(t, filepath.Join(experiment, study.FreezeDirName, "prereg-commit.txt"), []byte("9058fee\n"))
	mustWrite(t, filepath.Join(experiment, study.FreezeDirName, "prereg-commit.tsr"), []byte("token"))
	layout := study.Layout{Root: f.work}
	start := time.Date(2026, 11, 2, 21, 0, 0, 0, time.UTC)
	for attempt := 1; attempt <= 3; attempt++ {
		run := study.Run{Attempt: attempt, Outcome: study.OutcomeHarnessError, Reason: "the engine refused the container"}
		run.Task, run.Arm, run.Repetition, run.StartedAt = "boxoffice", ArmControl, 1, start.Add(time.Duration(attempt)*time.Hour)
		mustJSON(t, filepath.Join(phase.Records(layout.RunDir(blockedRun, attempt)), record.MetaFile), run)
	}
	quota := study.Run{Attempt: 1, Outcome: study.OutcomeQuotaStop}
	quota.StartedAt = start
	mustJSON(t, filepath.Join(phase.Records(layout.RunDir(countedRun, 1)), record.MetaFile), quota)
	f.writeCounted(t, layout.RunDir(countedRun, 2))
	mustWrite(t, layout.Deviations(), []byte(study.DeviationsTitle+"\n"+protocol+"\n"))
	mustWrite(t, filepath.Join(layout.Timestamps(), "2026-11-03-resumed.txt"), []byte(protocol+"\n"))
	mustWrite(t, filepath.Join(layout.Timestamps(), "2026-11-03-resumed.tsr"), []byte("token"))
	f.options = Options{
		Repo: f.repo, Config: filepath.Join(experiment, "loop-1.json"), Work: f.work, Home: f.home,
		Now: start.Add(time.Hour * 5), Imprints: fakeImprints{},
	}
	return f
}

// writeCounted writes the counted attempt from the demonstration's recording,
// with the analysis files, a stop report and a stderr that names the machine's
// paths.
func (f fixture) writeCounted(t *testing.T, dir string) {
	t.Helper()
	records := phase.Records(dir)
	for _, name := range []string{record.StreamFile, record.TimesFile, record.HooksFile, record.FinalGateFile, record.ChangeFile} {
		data, err := os.ReadFile(filepath.Join(recording, name))
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(records, name), data)
	}
	run := study.Run{Attempt: 2, Outcome: study.OutcomeCompleted}
	run.Task, run.Arm, run.Repetition = "boxoffice", ArmTreatment, 1
	run.StartedAt, run.EndedAt = time.Date(2026, 10, 25, 21, 0, 0, 0, time.UTC), time.Date(2026, 10, 25, 21, 9, 30, 0, time.UTC)
	mustJSON(t, filepath.Join(records, record.MetaFile), run)
	mustWrite(t, filepath.Join(records, record.StderrFile), []byte("cwd "+filepath.Join(dir, "project")+" home "+f.home+"/x\n"))
	mustWrite(t, filepath.Join(records, study.StopsDir, "2", study.StopReportFile), []byte(`{"verdict":"fail","exit_code":1,"findings":[{"category":"surviving_valid_mutant","blocking":true,"requirement_id":"BOX-2","module":"musthold-dp"}]}`))
	mustJSON(t, filepath.Join(records, study.OracleFile), study.Oracle{Defective: false})
	mustJSON(t, filepath.Join(records, study.HeldOutFile), study.HeldOut{study.DirFinal: {Killed: map[string]bool{"BOX-1/1": true}}})
	mustJSON(t, filepath.Join(records, study.DetectorsFile), study.Detection{})
}

// analyzed runs the verb on a fresh fixture and returns its results directory.
func analyzed(t *testing.T) (fixture, string) {
	t.Helper()
	f := newFixture(t)
	if err := Analyze(context.Background(), f.options, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	return f, f.options.Results()
}

func TestAnalyze(t *testing.T) {
	t.Run("the summary holds every run of the order with its attempts, facts and digests [verifies SP-2603~1]", func(t *testing.T) {
		f, results := analyzed(t)
		var summary Summary
		data, err := os.ReadFile(filepath.Join(results, SummaryFile))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &summary); err != nil {
			t.Fatal(err)
		}
		if len(summary.Runs) != 2 || summary.Runs[0].State != study.OutcomeHarnessBlocked || len(summary.Runs[0].Attempts) != 3 ||
			summary.Runs[1].Facts == nil || len(summary.Runs[1].Attempts) != 2 || summary.Runs[1].Facts.BlockCounts["finding"] != 1 {
			t.Fatalf("runs %+v", summary.Runs)
		}
		if summary.Digests[study.InputsFile] == "" || summary.Digests["work/runs/boxoffice-treatment-1-2/records/stream.jsonl"] == "" ||
			summary.Provenance.Tags[0].Time == nil || summary.Provenance.Tags[1].Time != nil || summary.Provenance.Dropped[0].Domain != "water bill" || summary.Provenance.Between != nil {
			t.Fatalf("summary %+v %+v", summary.Digests, summary.Provenance)
		}
		if _, err := os.Stat(filepath.Join(phase.Records(study.Layout{Root: f.work}.RunDir(countedRun, 2)), study.FactsFile)); err != nil {
			t.Fatal(err)
		}
		report, _ := os.ReadFile(filepath.Join(results, ReportFile))
		if !strings.HasPrefix(string(report), "# loop-1 results") || !strings.Contains(string(report), "harness-blocked") ||
			!strings.Contains(string(report), "- commits between the tags: not recorded in `inputs.json`") {
			t.Fatalf("report\n%s", report)
		}
		if strings.Contains(string(data), f.home) {
			t.Fatal("summary.json names the home directory")
		}
	})
	t.Run("the results hold each counted run's records with placeholders, its transcript, the deviation log and the sums [verifies SP-2605~1]", func(t *testing.T) {
		_, results := analyzed(t)
		kept := filepath.Join(results, RunsDir, countedRun)
		for _, name := range []string{record.MetaFile, study.FactsFile, study.OracleFile, record.TranscriptMarkdown, record.TranscriptHTML,
			filepath.Join(RawDir, record.StreamFile), filepath.Join(RawDir, study.StopsDir, "2", study.StopReportFile)} {
			if _, err := os.Stat(filepath.Join(kept, name)); err != nil {
				t.Errorf("missing %s", name)
			}
		}
		stderr, _ := os.ReadFile(filepath.Join(kept, RawDir, record.StderrFile))
		if string(stderr) != "cwd <run>/project home ~/x\n" {
			t.Fatalf("stderr %q", stderr)
		}
		sums, _ := os.ReadFile(filepath.Join(results, SumsFile))
		stream, _ := os.ReadFile(filepath.Join(kept, RawDir, record.StreamFile))
		digest := sha256.Sum256(stream)
		if !strings.Contains(string(sums), hex.EncodeToString(digest[:])+"  runs/boxoffice-treatment-1/raw/stream.jsonl\n") ||
			strings.Contains(string(sums), "run.json") || strings.Count(string(sums), "\n") != 5 {
			t.Fatalf("sums\n%s", sums)
		}
		for _, name := range []string{study.DeviationsFile, filepath.Join(study.DeviationTimestampsDir, "2026-11-03-resumed.tsr")} {
			if _, err := os.Stat(filepath.Join(results, name)); err != nil {
				t.Errorf("missing %s", name)
			}
		}
	})
	t.Run("the verb refuses before the run phase ends and writes nothing [verifies SP-2583~1]", func(t *testing.T) {
		f := newFixture(t)
		if err := os.RemoveAll(study.Layout{Root: f.work}.RunDir(blockedRun, 3)); err != nil {
			t.Fatal(err)
		}
		err := Analyze(context.Background(), f.options, &bytes.Buffer{})
		if !errors.Is(err, phase.ErrOpen) || !strings.Contains(err.Error(), blockedRun) {
			t.Fatalf("got %v", err)
		}
		if _, err := os.Stat(f.options.Results()); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("results were written")
		}
		f.options.Now = f.options.Now.Add(phase.CalendarCap)
		if err := Analyze(context.Background(), f.options, &bytes.Buffer{}); err != nil {
			t.Fatalf("at the cap: %v", err)
		}
	})
	t.Run("the verb refuses a deviation whose imprint differs [verifies SP-2585~1]", func(t *testing.T) {
		f := newFixture(t)
		f.options.Imprints = fakeImprints{wrong: true}
		if err := Analyze(context.Background(), f.options, &bytes.Buffer{}); !errors.Is(err, stamp.ErrDeviationStamps) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("Main reports a refusal and a usage error [verifies SP-2583~1]", func(t *testing.T) {
		f := newFixture(t)
		var stdout, stderr bytes.Buffer
		args := []string{"--repo", f.repo, "--config", f.options.Config, "--work", filepath.Join(f.home, "empty")}
		if code := Main(context.Background(), args, &stdout, &stderr); code != exitRefused || !strings.Contains(stderr.String(), "run phase") {
			t.Fatalf("exit %d: %s", code, stderr.String())
		}
		if code := Main(context.Background(), []string{"--nope"}, &stdout, &stderr); code != exitUsage {
			t.Fatalf("exit %d", code)
		}
	})
}
