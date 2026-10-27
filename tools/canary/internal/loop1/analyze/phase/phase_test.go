package phase

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

var start = time.Date(2026, 11, 2, 21, 0, 0, 0, time.UTC)

func attempts(outcomes ...study.Outcome) []Attempt {
	var list []Attempt
	for index, outcome := range outcomes {
		run := study.Run{Attempt: index + 1, Outcome: outcome}
		run.StartedAt = start.Add(time.Duration(index) * time.Hour)
		list = append(list, Attempt{Number: index + 1, Run: run})
	}
	return list
}

func TestStateOf(t *testing.T) {
	q, h, c := study.OutcomeQuotaStop, study.OutcomeHarnessError, study.OutcomeCompleted
	t.Run("a counted attempt counts the run, after any void attempts [verifies SP-2580~1]", func(t *testing.T) {
		state := StateOf("r", attempts(q, h, q, q, study.OutcomeCappedTime), false)
		if state.State != study.OutcomeCappedTime || state.Counted != 4 {
			t.Fatalf("got %+v", state)
		}
		if got, ok := state.CountedAttempt(); !ok || got.Number != 5 {
			t.Fatalf("counted %+v", got)
		}
	})
	t.Run("three harness errors block the run and quota stops never count [verifies SP-2580~1]", func(t *testing.T) {
		if got := StateOf("r", attempts(h, q, h, q, h), false).State; got != study.OutcomeHarnessBlocked {
			t.Fatalf("got %s", got)
		}
		if got := StateOf("r", attempts(q, q, q, q, h, h), false); got.State != "" || got.Closed() {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("at the cap a run takes the state of its last attempt [verifies SP-2580~1]", func(t *testing.T) {
		cases := map[study.Outcome][]Attempt{
			study.OutcomeNotRun:         nil,
			study.OutcomeQuotaBlocked:   attempts(h, q),
			study.OutcomeHarnessBlocked: attempts(q, h),
			c:                           attempts(q, c),
		}
		for want, list := range cases {
			if got := StateOf("r", list, true).State; got != want {
				t.Errorf("want %s, got %s", want, got)
			}
		}
	})
}

func TestJudge(t *testing.T) {
	order := []string{"a-control-1", "a-treatment-1", "b-control-1"}
	t.Run("the phase ends when every run is counted or blocked [verifies SP-2581~1]", func(t *testing.T) {
		all := map[string][]Attempt{
			"a-control-1":   attempts(study.OutcomeCompleted),
			"a-treatment-1": attempts(study.OutcomeHarnessError, study.OutcomeHarnessError, study.OutcomeHarnessError),
			"b-control-1":   attempts(study.OutcomeQuotaStop, study.OutcomeCappedMessages),
		}
		got := Judge(order, all, start.Add(time.Hour))
		if !got.Ended || got.CapReached || len(got.Open) != 0 || got.Require() != nil {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("an open run keeps the phase open until 28 days after the first start [verifies SP-2581~1]", func(t *testing.T) {
		some := map[string][]Attempt{
			"a-control-1":   attempts(study.OutcomeCompleted),
			"a-treatment-1": attempts(study.OutcomeQuotaStop),
		}
		before := Judge(order, some, start.Add(CalendarCap-time.Second))
		if before.Ended || strings.Join(before.Open, ",") != "a-treatment-1,b-control-1" {
			t.Fatalf("got %+v", before)
		}
		err := before.Require()
		if !errors.Is(err, ErrOpen) || !strings.Contains(err.Error(), "b-control-1") || !strings.Contains(err.Error(), "2026-11-30T21:00:00Z") {
			t.Fatalf("got %v", err)
		}
		at := Judge(order, some, start.Add(CalendarCap))
		if !at.Ended || !at.CapReached || len(at.Open) != 0 {
			t.Fatalf("got %+v", at)
		}
		if at.Runs[1].State != study.OutcomeQuotaBlocked || at.Runs[2].State != study.OutcomeNotRun {
			t.Fatalf("states %+v", at.Runs)
		}
	})
	t.Run("before the first attempt the phase is open and has no cap [verifies SP-2581~1]", func(t *testing.T) {
		got := Judge(order, nil, start.Add(100*CalendarCap))
		if got.Ended || !got.Cap.IsZero() || len(got.Open) != 3 {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestLoad(t *testing.T) {
	t.Run("attempts are read from the directories named after the runs [verifies SP-2582~1]", func(t *testing.T) {
		root := t.TempDir()
		layout := study.Layout{Root: root}
		write := func(run string, attempt int, outcome study.Outcome) {
			dir := Records(layout.RunDir(run, attempt))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(study.Run{Attempt: attempt, Outcome: outcome})
			if err := os.WriteFile(filepath.Join(dir, record.MetaFile), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		write("box-office-control-1", 2, study.OutcomeCompleted)
		write("box-office-control-1", 10, study.OutcomeCompleted)
		write("box-office-control-1", 1, study.OutcomeQuotaStop)
		write("box-office-control-10", 1, study.OutcomeCompleted)
		for _, stray := range []string{"box-office-control-1-x", "box-office-control-1-01", "other-1"} {
			if err := os.MkdirAll(filepath.Join(root, study.RunsDir, stray), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(layout.RunDir("box-office-treatment-1", 1), 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := Load(layout, []string{"box-office-control-1", "box-office-treatment-1"})
		if err != nil {
			t.Fatal(err)
		}
		list := got["box-office-control-1"]
		if len(got) != 2 || len(list) != 3 || list[0].Number != 1 || list[1].Number != 2 || list[2].Number != 10 ||
			list[0].Run.Outcome != study.OutcomeQuotaStop {
			t.Fatalf("got %+v", got)
		}
		if pending := got["box-office-treatment-1"]; len(pending) != 1 || pending[0].Run.Outcome != "" {
			t.Fatalf("an attempt without run.json has no outcome: %+v", pending)
		}
	})
	t.Run("a work directory with no runs holds no attempt [verifies SP-2582~1]", func(t *testing.T) {
		got, err := Load(study.Layout{Root: t.TempDir()}, []string{"a"})
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v %v", got, err)
		}
	})
}
