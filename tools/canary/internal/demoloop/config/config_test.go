package config_test

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
)

const valid = `{
  "tasks": [
    {"name": "boxoffice", "fixture": "demo/loop/fixture/boxoffice", "prompt": "Do the task.",
     "operations": {"ticket_price": {"python": "boxoffice.prices:ticket_price"}}},
    {"name": "parcel", "fixture": "discpower/fixtures/prove/parcel", "prompt": "Do another task."}
  ],
  "arms": ["traceability", "four-proofs"],
  "repetitions": 3,
  "seed": 7,
  "model": "claude-sonnet-5-5",
  "caps": {"assistant_messages": 100, "seconds": 600, "budget_usd": "2.50"},
  "allowed_tools": ["Bash(pytest *)"]
}`

func TestParse(t *testing.T) {
	t.Run("every parameter of the runs comes from the document [verifies SP-2400~2]", func(t *testing.T) {
		parsed, err := config.Parse([]byte(valid))
		if err != nil {
			t.Fatal(err)
		}
		if len(parsed.Tasks) != 2 || parsed.Tasks[0].Operations["ticket_price"]["python"] != "boxoffice.prices:ticket_price" {
			t.Errorf("tasks = %+v", parsed.Tasks)
		}
		if !slices.Equal(parsed.Arms, []config.Arm{config.ArmTraceability, config.ArmFourProofs}) || parsed.Repetitions != 3 || parsed.Seed != 7 {
			t.Errorf("arms %v, repetitions %d, seed %d", parsed.Arms, parsed.Repetitions, parsed.Seed)
		}
		if parsed.Model != "claude-sonnet-5-5" || parsed.Caps != (config.Caps{AssistantMessages: 100, Seconds: 600, BudgetUSD: "2.50"}) {
			t.Errorf("model %q, caps %+v", parsed.Model, parsed.Caps)
		}
		if !slices.Equal(parsed.AllowedTools, []string{"Bash(pytest *)"}) {
			t.Errorf("rules %v", parsed.AllowedTools)
		}
	})

	t.Run("the committed configuration of the demonstration parses [verifies SP-2400~2]", func(t *testing.T) {
		data, err := os.ReadFile("../../../../../demo/loop/demo.json")
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := config.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		if runs := parsed.Runs(); len(runs) != 1 || runs[0].Arm != config.ArmFourProofs {
			t.Errorf("runs = %+v", runs)
		}
	})

	t.Run("an unknown member, a second document and a broken document are refused [verifies SP-2400~2]", func(t *testing.T) {
		for name, document := range map[string]string{
			"unknown cap":     strings.Replace(valid, `"seconds": 600`, `"seconds": 600, "minutes": 10`, 1),
			"second document": valid + "\n{}",
			"broken":          `{"tasks": [`,
		} {
			if _, err := config.Parse([]byte(document)); !errors.Is(err, config.ErrMalformed) {
				t.Errorf("%s: error %v", name, err)
			}
		}
	})

	t.Run("every invalid member is named [verifies SP-2400~2]", func(t *testing.T) {
		document := `{"tasks": [{"name": "Bad Name", "fixture": "", "prompt": "", "operations": {"op": {}}}, {"name": "Bad Name", "fixture": "f", "prompt": "p"}],
		  "arms": ["everything"], "repetitions": 0, "model": "", "caps": {"assistant_messages": 0, "seconds": 0, "budget_usd": "2"}, "allowed_tools": [""]}`
		_, err := config.Parse([]byte(document))
		if !errors.Is(err, config.ErrInvalid) {
			t.Fatalf("error %v", err)
		}
		for _, member := range []string{"tasks[0].name", "tasks[0].fixture", "tasks[0].prompt", "tasks[0].operations.op", "tasks[1].name", "arms[0]", "repetitions", "model", "caps.assistant_messages", "caps.seconds", "caps.budget_usd", "allowed_tools[0]"} {
			if !strings.Contains(err.Error(), member) {
				t.Errorf("the error does not name %s: %v", member, err)
			}
		}
	})

	t.Run("a document without tasks or arms is refused [verifies SP-2400~2]", func(t *testing.T) {
		_, err := config.Parse([]byte(`{"repetitions": 1, "model": "m", "caps": {"assistant_messages": 1, "seconds": 1, "budget_usd": "1.00"}}`))
		if !errors.Is(err, config.ErrInvalid) || !strings.Contains(err.Error(), "tasks") || !strings.Contains(err.Error(), "arms") {
			t.Fatalf("error %v", err)
		}
	})
}

func TestRuns(t *testing.T) {
	parsed, err := config.Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("each task runs in each arm for each repetition, once [verifies SP-2400~2]", func(t *testing.T) {
		runs := parsed.Runs()
		if len(runs) != 2*2*3 {
			t.Fatalf("%d runs", len(runs))
		}
		names := map[string]bool{}
		for _, run := range runs {
			names[run.Name()] = true
		}
		if len(names) != len(runs) || !names["boxoffice-four-proofs-3"] || !names["parcel-traceability-1"] {
			t.Errorf("names %v", names)
		}
	})

	t.Run("the seed alone decides the order [verifies SP-2400~2]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			seeded := parsed
			seeded.Seed = rapid.Uint64().Draw(t, "seed")
			first, second := seeded.Runs(), seeded.Runs()
			if !slices.EqualFunc(first, second, func(a, b config.Run) bool { return a.Name() == b.Name() }) {
				t.Fatalf("two orders for seed %d", seeded.Seed)
			}
		})
		other := parsed
		other.Seed = parsed.Seed + 1
		same := slices.EqualFunc(parsed.Runs(), other.Runs(), func(a, b config.Run) bool { return a.Name() == b.Name() })
		if same {
			t.Error("two seeds gave the same order of twelve runs")
		}
	})
}
