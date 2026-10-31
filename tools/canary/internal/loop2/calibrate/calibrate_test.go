package calibrate

import (
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

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/decision"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
)

const (
	powerScript = "../../../../../experiments/loop-2/script/power"
	tasks       = 8
	repetitions = 2
)

// ready builds a ready entry of the coder on the phase with the control
// escapes among its 16 control runs.
func ready(coder string, phase Phase, escapes int) Entry {
	e := Entry{Coder: coder, Phase: phase, Replay: Replay{Loop1Trees: Loop1FinalTrees}}
	for task := range tasks {
		name := fmt.Sprintf("t%d", task)
		for repetition := range repetitions {
			index := task*repetitions + repetition
			e.Runs = append(e.Runs,
				Run{Run: fmt.Sprintf("%s-control-%d", name, repetition+1), Task: name, Arm: analyze.ArmControl, Accepted: true, Defective: index < escapes, Refusal: record.CauseNone},
				Run{Run: fmt.Sprintf("%s-treatment-%d", name, repetition+1), Task: name, Arm: analyze.ArmTreatment, Accepted: true, Refusal: record.CauseNone},
			)
		}
		e.PositiveControl = append(e.PositiveControl, Violation{Task: name, Violation: name + "/1", Criterion: name, FixtureTestsPass: true, Refused: true, NamesCriterion: true})
		e.Replay.References = append(e.Replay.References, Reference{Task: name, Passes: true})
	}
	return e
}

func refuseClean(e Entry, n int, cause record.Cause) Entry {
	for index := range e.Runs {
		if n > 0 && e.Runs[index].Arm == analyze.ArmTreatment && !e.Runs[index].Defective {
			e.Runs[index].Accepted, e.Runs[index].Refusal = false, cause
			n--
		}
	}
	return e
}

func TestJudge(t *testing.T) {
	t.Run("a coder is ready when every readiness rule holds, and each failure names its rule [verifies SP-2821~1]", func(t *testing.T) {
		base := ready(Sonnet, PhaseC1, 6)
		if got := Judge(base); !got.Ready || got.ControlEscapes != (facts.Fraction{K: 6, N: 16}) || got.PositiveControl != (facts.Fraction{K: 8, N: 8}) {
			t.Fatalf("ready: %+v", got)
		}
		if got := Judge(refuseClean(ready(Sonnet, PhaseC1, 6), 1, record.CauseFinding)); !got.Ready || got.RefusedClean != (facts.Fraction{K: 1, N: 16}) {
			t.Fatalf("one refused clean change: %+v", got)
		}
		cases := []struct {
			name   string
			entry  func() Entry
			reason string
		}{
			{"two refused clean changes", func() Entry { return refuseClean(ready(Sonnet, PhaseC1, 6), 2, record.CauseFinding) }, ReasonRefusedClean},
			{"one clean change refused for pending proofs", func() Entry { return refuseClean(ready(Sonnet, PhaseC1, 6), 1, record.CausePending) }, ReasonPendingRefusal},
			{"5 of 8 violations caught", func() Entry {
				e := ready(Sonnet, PhaseC1, 6)
				for index := range 3 {
					e.PositiveControl[index].NamesCriterion = false
				}
				return e
			}, ReasonPositiveControl},
			{"no violation the fixture's tests pass", func() Entry {
				e := ready(Sonnet, PhaseC1, 6)
				for index := range e.PositiveControl {
					e.PositiveControl[index].FixtureTestsPass = false
				}
				return e
			}, ReasonPositiveControl},
			{"a firing on loop-1's trees", func() Entry { e := ready(Sonnet, PhaseC1, 6); e.Replay.Loop1Firings = 1; return e }, ReasonLoop1Replay},
			{"35 loop-1 trees replayed", func() Entry { e := ready(Sonnet, PhaseC1, 6); e.Replay.Loop1Trees = 35; return e }, ReasonLoop1Replay},
			{"a firing on a reference", func() Entry { e := ready(Sonnet, PhaseC1, 6); e.Replay.References[2].Firings = 1; return e }, ReasonReferences},
			{"7 of 8 references pass", func() Entry { e := ready(Sonnet, PhaseC1, 6); e.Replay.References[0].Passes = false; return e }, ReasonReferences},
			{"7 references listed", func() Entry { e := ready(Sonnet, PhaseC1, 6); e.Replay.References = e.Replay.References[1:]; return e }, ReasonReferences},
		}
		for _, c := range cases {
			got := Judge(c.entry())
			if got.Ready || !slices.Contains(got.Reasons, c.reason) {
				t.Errorf("%s: %+v", c.name, got)
			}
		}
		// 6 of 8 caught is exactly 3/4.
		e := ready(Sonnet, PhaseC1, 6)
		e.PositiveControl[0].Refused, e.PositiveControl[1].Refused = false, false
		if !Judge(e).Ready {
			t.Fatal("3/4 of the violations caught is ready")
		}
	})
}

func notReady(coder string, phase Phase, escapes int) Entry {
	e := ready(coder, phase, escapes)
	e.Replay.Loop1Firings = 1
	return e
}

func TestSelect(t *testing.T) {
	t.Run("after C1: back to the fixes without a ready coder, Sonnet then Haiku with 5 control escapes, else C2 [verifies SP-2822~1]", func(t *testing.T) {
		cases := []struct {
			name   string
			sonnet Entry
			haiku  Entry
			want   Decision
			coder  string
		}{
			{"no ready coder", notReady(Sonnet, PhaseC1, 9), notReady(Haiku, PhaseC1, 9), DecisionReturnToFixes, ""},
			{"Sonnet ready with 5 escapes", ready(Sonnet, PhaseC1, 5), ready(Haiku, PhaseC1, 9), DecisionSelected, Sonnet},
			{"Sonnet ready with 4 escapes, Haiku with 5", ready(Sonnet, PhaseC1, 4), ready(Haiku, PhaseC1, 5), DecisionSelected, Haiku},
			{"Sonnet not ready, Haiku with 7", notReady(Sonnet, PhaseC1, 9), ready(Haiku, PhaseC1, 7), DecisionSelected, Haiku},
			{"both ready below 5 escapes", ready(Sonnet, PhaseC1, 4), ready(Haiku, PhaseC1, 4), DecisionCallC2, ""},
			{"Haiku ready below 5, Sonnet not ready", notReady(Sonnet, PhaseC1, 9), ready(Haiku, PhaseC1, 1), DecisionCallC2, ""},
		}
		for _, c := range cases {
			got, err := Select(Calibration{Coders: []Entry{c.sonnet, c.haiku}})
			if err != nil || got.Decision != c.want || got.Coder != c.coder {
				t.Errorf("%s: %+v, %v", c.name, got, err)
			}
		}
	})
	t.Run("after C2: readiness on C2, back to the fixes, the base-rate stop on 0/32, else the most pooled escapes with Sonnet on a tie [verifies SP-2823~1]", func(t *testing.T) {
		c1 := []Entry{ready(Sonnet, PhaseC1, 0), ready(Haiku, PhaseC1, 3)}
		cases := []struct {
			name    string
			c1, c2  []Entry
			want    Decision
			coder   string
			outcome decision.Outcome
		}{
			{"no coder ready on C2", c1, []Entry{notReady(Sonnet, PhaseC2, 9), notReady(Haiku, PhaseC2, 9)}, DecisionReturnToFixes, "", ""},
			{"the ready coder has 0/32", c1, []Entry{ready(Sonnet, PhaseC2, 0), notReady(Haiku, PhaseC2, 9)}, DecisionBaseRateStop, "", decision.NoUsefulGain},
			{"every ready coder has 0/32", []Entry{ready(Sonnet, PhaseC1, 0), ready(Haiku, PhaseC1, 0)}, []Entry{ready(Sonnet, PhaseC2, 0), ready(Haiku, PhaseC2, 0)}, DecisionBaseRateStop, "", decision.NoUsefulGain},
			{"Haiku pools 3 + 1 against Sonnet's 0 + 2", c1, []Entry{ready(Sonnet, PhaseC2, 2), ready(Haiku, PhaseC2, 1)}, DecisionSelected, Haiku, ""},
			{"a tie goes to Sonnet", c1, []Entry{ready(Sonnet, PhaseC2, 3), ready(Haiku, PhaseC2, 0)}, DecisionSelected, Sonnet, ""},
			{"a coder not ready on C2 is out whatever it pooled", c1, []Entry{ready(Sonnet, PhaseC2, 1), notReady(Haiku, PhaseC2, 9)}, DecisionSelected, Sonnet, ""},
		}
		for _, c := range cases {
			got, err := Select(Calibration{Coders: append(append([]Entry{}, c.c1...), c.c2...)})
			if err != nil || got.Decision != c.want || got.Coder != c.coder || got.Outcome != c.outcome {
				t.Errorf("%s: %+v, %v", c.name, got, err)
			}
		}
		got, _ := Select(Calibration{Coders: append(append([]Entry{}, c1...), ready(Sonnet, PhaseC2, 2), ready(Haiku, PhaseC2, 1))})
		if got.Pooled[Haiku] != (facts.Fraction{K: 4, N: 32}) || len(got.Readiness) != 4 {
			t.Fatalf("pooled %+v", got.Pooled)
		}
	})
	t.Run("C1 calling for C2 without C2 data calls C2; C2 data C1 does not call for, or a missing entry, is refused [verifies SP-2823~1]", func(t *testing.T) {
		got, err := Select(Calibration{Coders: []Entry{ready(Sonnet, PhaseC1, 1), ready(Haiku, PhaseC1, 1)}})
		if err != nil || got.Decision != DecisionCallC2 {
			t.Fatalf("%+v, %v", got, err)
		}
		if _, err := Select(Calibration{Coders: []Entry{ready(Sonnet, PhaseC1, 6), ready(Haiku, PhaseC1, 1), ready(Sonnet, PhaseC2, 1), ready(Haiku, PhaseC2, 1)}}); !errors.Is(err, ErrUnexpectedC2) {
			t.Fatalf("unexpected C2: %v", err)
		}
		if _, err := Select(Calibration{Coders: []Entry{ready(Sonnet, PhaseC1, 6)}}); !errors.Is(err, ErrMissingEntry) {
			t.Fatalf("missing Haiku: %v", err)
		}
	})
}

type fakeReacher struct{ escapes, runs int }

func (f *fakeReacher) Reach(_ context.Context, escapes, runs int) (string, error) {
	f.escapes, f.runs = escapes, runs
	return "reach output\n", nil
}

func writeCalibration(t *testing.T, c Calibration) string {
	t.Helper()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "calibration.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReach(t *testing.T) {
	t.Run("the power script's reach output is recorded verbatim beside the selection [verifies SP-2824~1]", func(t *testing.T) {
		got, err := PowerScript{Path: powerScript}.Reach(context.Background(), 6, 16)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(got, "shrunk control rate 0.389, 24 tasks, 8 runs per task and arm\n") || !strings.Contains(got, "no effect: USEFUL GAIN") {
			t.Fatalf("output %q", got)
		}
		if _, err := (PowerScript{Path: filepath.Join(t.TempDir(), "absent")}).Reach(context.Background(), 1, 16); err == nil {
			t.Fatal("an absent script reached")
		}
	})
	t.Run("the verb reaches at the selected coder's pooled control escapes and writes both files [verifies SP-2824~1]", func(t *testing.T) {
		path := writeCalibration(t, Calibration{Coders: []Entry{ready(Sonnet, PhaseC1, 4), ready(Haiku, PhaseC1, 7)}})
		out := t.TempDir()
		reacher := &fakeReacher{}
		selection, err := Calibrate(context.Background(), path, out, reacher)
		if err != nil || selection.Coder != Haiku || reacher.escapes != 7 || reacher.runs != 16 {
			t.Fatalf("%+v, %v, reacher %+v", selection, err, reacher)
		}
		reach, err := os.ReadFile(filepath.Join(out, ReachFile))
		if err != nil || string(reach) != "reach output\n" {
			t.Fatalf("reach.txt %q, %v", reach, err)
		}
		var written Selection
		data, _ := os.ReadFile(filepath.Join(out, SelectionFile))
		if err := json.Unmarshal(data, &written); err != nil || written.Coder != Haiku {
			t.Fatalf("selection.json %s, %v", data, err)
		}
	})
	t.Run("without a selected coder no reach is run [verifies SP-2824~1]", func(t *testing.T) {
		path := writeCalibration(t, Calibration{Coders: []Entry{ready(Sonnet, PhaseC1, 1), ready(Haiku, PhaseC1, 1)}})
		out := t.TempDir()
		reacher := &fakeReacher{}
		if _, err := Calibrate(context.Background(), path, out, reacher); err != nil || reacher.runs != 0 {
			t.Fatalf("%v, %+v", err, reacher)
		}
		if _, err := os.Stat(filepath.Join(out, ReachFile)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("reach.txt written without a coder")
		}
	})
	t.Run("the verb's usage errors exit 2 and a refusal exits 1 [verifies SP-2824~1]", func(t *testing.T) {
		command := Command{Reacher: func(string) Reacher { return &fakeReacher{} }}
		if command.Main(context.Background(), []string{"--out", t.TempDir()}, io.Discard, io.Discard) != exitUsage {
			t.Fatal("a missing --calibration")
		}
		if command.Main(context.Background(), []string{"--calibration", filepath.Join(t.TempDir(), "absent.json"), "--out", t.TempDir()}, io.Discard, io.Discard) != exitRefused {
			t.Fatal("an absent calibration.json")
		}
	})
}
