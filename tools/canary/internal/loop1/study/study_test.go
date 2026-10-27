package study

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const boxofficeTask = `{
  "name": "boxoffice",
  "fixture": "demo/loop/fixture/boxoffice",
  "spec": "specs/tickets.md",
  "source": "boxoffice/prices.py",
  "criterion": "BOX-4",
  "prompt": "Implement requirement BOX-4 of specs/tickets.md.",
  "operations": {"ticket_price": {"python": "boxoffice.prices:ticket_price"}},
  "interface": {"module": "boxoffice.prices", "operation": "ticket_price", "signature": "ticket_price(*, age: int) -> int"},
  "truth": {
    "reference": "tools/canary/internal/demoloop/fixturegate/testdata/task/boxoffice",
    "violations": "experiments/loop-1/tasks/boxoffice/violations",
    "refactors": "experiments/loop-1/tasks/boxoffice/refactors",
    "oracle": "experiments/loop-1/tasks/boxoffice/oracle"
  }
}`

func TestTask(t *testing.T) {
	t.Run("every member of a task comes from its description [verifies SP-2500~1]", func(t *testing.T) {
		task, err := ParseTask([]byte(boxofficeTask))
		if err != nil {
			t.Fatal(err)
		}
		if task.Name != "boxoffice" || task.Criterion != "BOX-4" || task.Spec != "specs/tickets.md" ||
			task.Interface.Operation != "ticket_price" || task.Truth.Oracle != "experiments/loop-1/tasks/boxoffice/oracle" ||
			task.Operations["ticket_price"]["python"] != "boxoffice.prices:ticket_price" {
			t.Fatalf("parsed %+v", task)
		}
	})
	t.Run("an unknown member and a second document are refused [verifies SP-2500~1]", func(t *testing.T) {
		for _, doc := range []string{
			strings.Replace(boxofficeTask, `"name"`, `"nmae": "x", "name"`, 1),
			boxofficeTask + boxofficeTask,
		} {
			if _, err := ParseTask([]byte(doc)); !errors.Is(err, ErrMalformedTask) {
				t.Fatalf("got %v", err)
			}
		}
	})
	t.Run("every member at fault is named [verifies SP-2500~1]", func(t *testing.T) {
		doc := strings.NewReplacer(
			`"demo/loop/fixture/boxoffice"`, `"/abs/fixture"`,
			`"specs/tickets.md"`, `"../specs/tickets.md"`,
			`"BOX-4"`, `"box four"`,
			`"Implement requirement BOX-4 of specs/tickets.md."`, `""`,
			`"ticket_price(*, age: int) -> int"`, `"price(age)"`,
		).Replace(boxofficeTask)
		_, err := ParseTask([]byte(doc))
		if !errors.Is(err, ErrInvalidTask) {
			t.Fatalf("got %v", err)
		}
		for _, member := range []string{"fixture:", "spec:", "criterion:", "prompt:", "interface.signature:"} {
			if !strings.Contains(err.Error(), member) {
				t.Errorf("%q not named in %v", member, err)
			}
		}
	})
	t.Run("an empty operation binding is refused [verifies SP-2500~1]", func(t *testing.T) {
		doc := strings.Replace(boxofficeTask, `{"ticket_price": {"python": "boxoffice.prices:ticket_price"}}`, `{"ticket_price": {}}`, 1)
		if _, err := ParseTask([]byte(doc)); err == nil || !strings.Contains(err.Error(), "operations.ticket_price") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("every committed task directory holds a task description [verifies SP-2500~1]", func(t *testing.T) {
		entries, err := os.ReadDir("../../../../../experiments/loop-1/tasks")
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join("../../../../../experiments/loop-1/tasks", entry.Name(), TaskFile)); err != nil {
				t.Errorf("%s: %v", entry.Name(), err)
			}
		}
	})
	t.Run("the committed task descriptions parse [verifies SP-2500~1]", func(t *testing.T) {
		paths, _ := filepath.Glob("../../../../../experiments/loop-1/tasks/*/" + TaskFile)
		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseTask(data); err != nil {
				t.Errorf("%s: %v", p, err)
			}
		}
	})
}

func TestOutcome(t *testing.T) {
	t.Run("counted, void and blocked outcomes [verifies SP-2501~1]", func(t *testing.T) {
		for _, o := range []Outcome{OutcomeCompleted, OutcomeCappedMessages, OutcomeCappedTime, OutcomeCappedBudget, OutcomeCappedMemory} {
			if !o.Counted() || o.Void() {
				t.Errorf("%s should be counted", o)
			}
		}
		if OutcomeCompleted.Capped() || !OutcomeCappedTime.Capped() {
			t.Error("only a capped outcome is capped")
		}
		for _, o := range []Outcome{OutcomeQuotaStop, OutcomeHarnessError} {
			if o.Counted() || !o.Void() {
				t.Errorf("%s should be void", o)
			}
		}
		if OutcomeQuotaStop.Blocked() != OutcomeQuotaBlocked || OutcomeHarnessError.Blocked() != OutcomeHarnessBlocked {
			t.Error("a third void attempt blocks the run after its cause")
		}
		for _, o := range []Outcome{OutcomeQuotaBlocked, OutcomeHarnessBlocked, OutcomeNotRun} {
			if o.Counted() || o.Void() {
				t.Errorf("%s is neither counted nor rerun", o)
			}
		}
	})
}

func TestDeviations(t *testing.T) {
	at := time.Date(2026, 10, 28, 9, 30, 0, 0, time.UTC)
	entry := Deviation{Time: at, Kind: KindHarnessError, Scope: []string{"boxoffice-control-1"}, Section: "6.4",
		Says: "a preparation step fails", Happened: "the engine refused the container | exit 125"}
	t.Run("an entry is written as one line and read back [verifies SP-2502~1]", func(t *testing.T) {
		line, err := entry.Line()
		if err != nil {
			t.Fatal(err)
		}
		want := "- 2026-10-28T09:30:00Z | harness-error | boxoffice-control-1 | §6.4 | a preparation step fails | the engine refused the container | exit 125"
		if line != want {
			t.Fatalf("got %q", line)
		}
		read := ParseDeviations(DeviationsTitle + "\n\n" + line + "\n")
		if len(read) != 1 || read[0].Happened != entry.Happened || read[0].Kind != entry.Kind || !read[0].Time.Equal(at) || read[0].Raw != "" {
			t.Fatalf("read %+v", read)
		}
	})
	t.Run("a kind outside the set, a missing field and a separator are refused [verifies SP-2502~1]", func(t *testing.T) {
		for _, bad := range []Deviation{
			{Time: at, Kind: "typo", Scope: []string{ScopeNone}, Section: "2", Says: "a", Happened: "b"},
			{Time: at, Kind: KindProtocol, Scope: []string{ScopeNone}, Section: "2", Says: "", Happened: "b"},
			{Time: at, Kind: KindProtocol, Scope: []string{ScopeNone}, Section: "2", Says: "a | b", Happened: "c"},
			{Time: at, Kind: KindProtocol, Scope: []string{ScopeNone}, Section: "2", Says: "a", Happened: "two\nlines"},
		} {
			if _, err := bad.Line(); !errors.Is(err, ErrDeviation) {
				t.Errorf("%+v: got %v", bad, err)
			}
		}
	})
	t.Run("a line of another shape is a protocol entry with scope all [verifies SP-2502~1]", func(t *testing.T) {
		read := ParseDeviations("# Deviations\nthe operator paused the batch\n- not | an | entry")
		if len(read) != 2 {
			t.Fatalf("read %+v", read)
		}
		for _, d := range read {
			if d.Kind != KindProtocol || len(d.Scope) != 1 || d.Scope[0] != ScopeAll || d.Raw == "" {
				t.Errorf("read %+v", d)
			}
		}
	})
	t.Run("only protocol and token-exposed entries are timestamped [verifies SP-2502~1]", func(t *testing.T) {
		for _, k := range Kinds() {
			if k.Timestamped() != (k == KindProtocol || k == KindTokenExposed) {
				t.Errorf("%s", k)
			}
		}
	})
}

func TestRecords(t *testing.T) {
	t.Run("defective, flaky and clean oracle tests [verifies SP-2503~1]", func(t *testing.T) {
		cases := map[[OracleRuns]bool][2]bool{
			{true, true, false}:   {true, false},
			{true, true, true}:    {true, false},
			{false, true, false}:  {false, true},
			{false, false, false}: {false, false},
		}
		for failed, want := range cases {
			test := OracleTest{Failed: failed}
			if test.Defective() != want[0] || test.Flaky() != want[1] {
				t.Errorf("%v: defective %v flaky %v", failed, test.Defective(), test.Flaky())
			}
		}
	})
	t.Run("the kill share counts the killed violations over all of them [verifies SP-2503~1]", func(t *testing.T) {
		m := KillMatrix{Killed: map[string]bool{"BOX-1/1": true, "BOX-1/2": false, "BOX-2/1": true}}
		if killed, total := m.Share(); killed != 2 || total != 3 {
			t.Fatalf("got %d/%d", killed, total)
		}
	})
}

func TestInputs(t *testing.T) {
	doc := `{"commits":{"prereg":"a","inputs":"b","between":[]},"egress_allowlist":["api.anthropic.com"],
	"quota_patterns":["usage limit"],"order":["boxoffice-control-1"],
	"tasks":[{"name":"boxoffice","base_commit":"c","equipped_tree":{"control":"d","treatment":"e"}}]}`
	t.Run("the members of the freeze are read and a task is found by name [verifies SP-2504~1]", func(t *testing.T) {
		in, err := ParseInputs([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		task, ok := in.Task("boxoffice")
		if !ok || task.BaseCommit != "c" || task.EquippedTree["treatment"] != "e" || in.EgressAllowlist[0] != "api.anthropic.com" || in.Order[0] != "boxoffice-control-1" {
			t.Fatalf("read %+v", in)
		}
		if _, ok := in.Task("library"); ok {
			t.Fatal("found a task the freeze does not hold")
		}
	})
	t.Run("an unknown member and a second document are refused [verifies SP-2504~1]", func(t *testing.T) {
		for _, bad := range []string{`{"orders":[]}`, `{}{}`} {
			if _, err := ParseInputs([]byte(bad)); !errors.Is(err, ErrMalformedInputs) {
				t.Errorf("%s: got %v", bad, err)
			}
		}
	})
}
