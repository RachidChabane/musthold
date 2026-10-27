package detect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

type fixedPhase bool

func (p fixedPhase) RunPhaseEnded(context.Context, string) (bool, error) { return bool(p), nil }

func wireHost(phase PhaseEnded) func(string) (Dependencies, error) {
	return func(string) (Dependencies, error) { return Dependencies{Runner: hostRunner{}, Phase: phase}, nil }
}

func setOutcome(t *testing.T, attempt string, outcome study.Outcome) {
	t.Helper()
	path := filepath.Join(attempt, RecordsDir, RunFile)
	var run runRecord
	if err := json.Unmarshal([]byte(readFile(t, path)), &run); err != nil {
		t.Fatal(err)
	}
	run.Outcome = outcome
	writeJSON(t, path, run)
}

func writeInputs(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inputs.json")
	writeJSON(t, path, testInputs)
	return path
}

func TestMainDetectsCountedAttemptsOnceTheRunPhaseEnded(t *testing.T) {
	requireHostTools(t)
	work := t.TempDir()
	runs := filepath.Join(work, study.RunsDir)
	counted := newAttempt(t, runs, "boxoffice-control-1-1", attemptSpec{end: removeSeniorTest})
	void := newAttempt(t, runs, "boxoffice-control-1-2", attemptSpec{})
	setOutcome(t, void, study.OutcomeQuotaStop)
	inputs := writeInputs(t)
	detectorsOf := func(attempt string) string {
		return filepath.Join(attempt, RecordsDir, study.DetectorsFile)
	}

	t.Run("[verifies SP-2579~1] refuses while the run phase is open", func(t *testing.T) {
		var stderr bytes.Buffer
		code := run(context.Background(), wireHost(fixedPhase(false)), []string{"--work", work, "--inputs", inputs}, &bytes.Buffer{}, &stderr)
		if code != exitRefused || !bytes.Contains(stderr.Bytes(), []byte(ErrRunPhaseOpen.Error())) {
			t.Fatalf("code %d, stderr %q", code, stderr.String())
		}
		if _, err := os.Stat(detectorsOf(counted)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("detectors.json was written: %v", err)
		}
	})

	t.Run("[verifies SP-2579~1] writes detectors.json for counted attempts only", func(t *testing.T) {
		code := run(context.Background(), wireHost(fixedPhase(true)), []string{"--work", work, "--inputs", inputs}, &bytes.Buffer{}, &bytes.Buffer{})
		if code != exitOK {
			t.Fatalf("code %d", code)
		}
		var detection study.Detection
		if err := json.Unmarshal([]byte(readFile(t, detectorsOf(counted))), &detection); err != nil {
			t.Fatal(err)
		}
		if len(hitsOf(detection, study.DetectorUncited)) != 1 || len(detection.Edits) != 1 {
			t.Fatalf("detection = %+v", detection)
		}
		if _, err := os.Stat(detectorsOf(void)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("the void attempt was detected: %v", err)
		}
	})

	t.Run("[verifies SP-2579~1] a usage error and an unwired verb", func(t *testing.T) {
		if code := run(context.Background(), wireHost(fixedPhase(true)), []string{"--unknown"}, &bytes.Buffer{}, &bytes.Buffer{}); code != exitUsage {
			t.Fatalf("code %d", code)
		}
		if code := run(context.Background(), nil, []string{"--work", work, "--inputs", inputs}, &bytes.Buffer{}, &bytes.Buffer{}); code != exitRefused {
			t.Fatalf("code %d", code)
		}
	})
}

func TestBoxExtractorRefusesAFailedHelper(t *testing.T) {
	failing := runnerFunc(func(context.Context, box.Command) (box.Result, error) {
		return box.Result{ExitCode: 1, Stderr: []byte("boom")}, nil
	})
	garbage := runnerFunc(func(context.Context, box.Command) (box.Result, error) {
		return box.Result{Stdout: []byte("not json")}, nil
	})
	t.Run("[verifies SP-2570~1] a failed helper run", func(t *testing.T) {
		if _, err := (BoxExtractor{Runner: failing}).Extract(context.Background(), "p", "c"); !errors.Is(err, ErrExtractFailed) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("[verifies SP-2570~1] output that is not facts", func(t *testing.T) {
		if _, err := (BoxExtractor{Runner: garbage}).Extract(context.Background(), "p", "c"); !errors.Is(err, ErrExtractOutput) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("[verifies SP-2570~1] the helper runs through the runner, isolated, over a read-only project", func(t *testing.T) {
		var seen box.Command
		spy := runnerFunc(func(_ context.Context, command box.Command) (box.Result, error) {
			seen = command
			return box.Result{Stdout: []byte("{}")}, nil
		})
		if _, err := (BoxExtractor{Runner: spy}).Extract(context.Background(), "/p", "abc"); err != nil {
			t.Fatal(err)
		}
		if seen.Argv[0] != "python3" || seen.Argv[1] != "-I" || seen.Argv[len(seen.Argv)-2] != "/p" || seen.Argv[len(seen.Argv)-1] != "abc" {
			t.Fatalf("argv = %q", seen.Argv)
		}
		if len(seen.Mounts) != 1 || seen.Mounts[0] != (box.Mount{Path: "/p", ReadOnly: true}) {
			t.Fatalf("mounts = %+v", seen.Mounts)
		}
	})
}

type runnerFunc func(context.Context, box.Command) (box.Result, error)

func (f runnerFunc) Run(ctx context.Context, command box.Command) (box.Result, error) {
	return f(ctx, command)
}
