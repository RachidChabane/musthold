package task_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/task"
)

// sample is a valid task description of the tier max, revising RATE-4 and
// adding RATE-12.
func sample() task.Task {
	t := task.WithStandardPaths(task.Task{
		Name: "parcel-rates", Tier: task.TierMax, Spec: "specs/rates.md", Package: "parcels",
		Criteria: []task.Criterion{{ID: "RATE-4", Kind: task.KindRevised}, {ID: "RATE-12", Kind: task.KindNew}},
		Operations: map[string]map[string]string{
			"quote": {"python": "parcels.rates:quote"}, "insure": {"python": "parcels.cover:insure"}, "refund": {"python": "parcels.rates:refund"},
		},
		Interface: []task.Operation{
			{Module: "parcels.rates", Operation: "quote", Signature: "quote(*, weight, zone)"},
			{Module: "parcels.cover", Operation: "insure", Signature: "insure(*, value)"},
			{Module: "parcels.rates", Operation: "refund", Signature: "refund(*, paid, days)"},
		},
	})
	t.Prompt = "Implement the revised requirement RATE-4 and the new requirement RATE-12 of specs/rates.md, heavier parcels and weekend surcharge, in parcels/, and update and add the tests that verify them. Tell me when the task is done."
	return t
}

func encode(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseTask(t *testing.T) {
	t.Run("a description of the loop-2 shape parses, with its truth paths in the task's directory [verifies SP-2900~1]", func(t *testing.T) {
		parsed, err := task.ParseTask(encode(t, sample()))
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Truth.SecondOracle != "experiments/loop-2/tasks/parcel-rates/oracle-2" || parsed.Fixture != "experiments/loop-2/tasks/parcel-rates/fixture" {
			t.Fatalf("%+v", parsed.Truth)
		}
		if revised, found := parsed.Revised(); !found || revised.ID != "RATE-4" {
			t.Fatalf("revised %+v", revised)
		}
	})

	cases := map[string]func(*task.Task){
		"tier":                   func(d *task.Task) { d.Tier = "easy" },
		"criteria":               func(d *task.Task) { d.Criteria = d.Criteria[:1] },
		"kind":                   func(d *task.Task) { d.Criteria[1].Kind = "removed" },
		"revised":                func(d *task.Task) { d.Criteria[1].Kind = task.KindRevised },
		"id":                     func(d *task.Task) { d.Criteria[0].ID = "rate4" },
		"interface":              func(d *task.Task) { d.Interface = d.Interface[:2]; delete(d.Operations, "refund") },
		"operations.refund":      func(d *task.Task) { d.Operations["refund"] = map[string]string{"python": "parcels.rates:quote"} },
		"interface[1].signature": func(d *task.Task) { d.Interface[1].Signature = "cover(value)" },
		"package":                func(d *task.Task) { d.Package = "../parcels" },
		"name":                   func(d *task.Task) { d.Name = "Parcel" },
		"prompt":                 func(d *task.Task) { d.Prompt = "" },
	}
	for member, spoil := range cases {
		t.Run("a description is refused naming "+member+" [verifies SP-2900~1]", func(t *testing.T) {
			d := sample()
			spoil(&d)
			_, err := task.ParseTask(encode(t, d))
			if !errors.Is(err, task.ErrInvalidTask) || !strings.Contains(err.Error(), member) {
				t.Fatalf("got %v", err)
			}
		})
	}

	t.Run("an unknown member and a second document are refused [verifies SP-2900~1]", func(t *testing.T) {
		data := encode(t, sample())
		withExtra := strings.Replace(string(data), `{"name"`, `{"extra":1,"name"`, 1)
		if _, err := task.ParseTask([]byte(withExtra)); !errors.Is(err, task.ErrMalformedTask) {
			t.Fatalf("got %v", err)
		}
		if _, err := task.ParseTask(append(data, data...)); !errors.Is(err, task.ErrMalformedTask) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestPrompt(t *testing.T) {
	t.Run("a revision task's prompt follows the revision wording [verifies SP-2901~1]", func(t *testing.T) {
		if err := sample().CheckPrompt(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a task of two new criteria follows loop-1's sentence for two criteria [verifies SP-2901~1]", func(t *testing.T) {
		d := sample()
		d.Criteria[0].Kind = task.KindNew
		d.Prompt = "Implement requirements RATE-4 and RATE-12 of specs/rates.md, heavier parcels and weekend surcharge, in parcels/, and add tests that verify them. Tell me when the task is done."
		if err := d.CheckPrompt(); err != nil {
			t.Fatal(err)
		}
		d.Criteria[0].Kind = task.KindRevised
		if err := d.CheckPrompt(); err == nil {
			t.Fatal("the new wording was accepted for a revision")
		}
	})

	t.Run("a statement longer than twelve words or on two lines is refused [verifies SP-2901~1]", func(t *testing.T) {
		d := sample()
		d.Prompt = strings.Replace(d.Prompt, "heavier parcels and weekend surcharge", "one two three four five six seven eight nine ten eleven twelve thirteen", 1)
		if err := d.CheckPrompt(); err == nil {
			t.Fatal("accepted")
		}
		d = sample()
		d.Prompt = strings.Replace(d.Prompt, "heavier parcels", "heavier\nparcels", 1)
		if err := d.CheckPrompt(); err == nil {
			t.Fatal("accepted")
		}
	})
}

func TestSize(t *testing.T) {
	fixture := map[string][]byte{
		"specs/rates.md":     []byte("# Rates\n\n- a\n"),
		"parcels/rates.py":   []byte("x = 1\n\n\ny = 2\n   \n"),
		"tests/test_a.py":    []byte("def test():\n    pass\n"),
		"pytest.ini":         []byte("[pytest]\ntestpaths = tests\n"),
		"specs/notes/old.md": []byte("not the spec\n"),
	}
	t.Run("the size counts the non-blank lines of the Python files and of the spec alone [verifies SP-2903~1]", func(t *testing.T) {
		if got := task.Lines(fixture, "specs/rates.md"); got != 6 {
			t.Fatalf("got %d", got)
		}
	})

	t.Run("a hard task holds 250 to 600 lines and a max task 400 to 600 [verifies SP-2903~1] [verifies SP-2923~1]", func(t *testing.T) {
		for _, c := range []struct {
			lines int
			tier  task.Tier
			fits  bool
		}{{249, task.TierHard, false}, {250, task.TierHard, true}, {600, task.TierHard, true}, {601, task.TierHard, false}, {399, task.TierMax, false}, {400, task.TierMax, true}} {
			if fits := task.SizeProblem(c.lines, c.tier) == ""; fits != c.fits {
				t.Errorf("%d lines at %s: fits %t", c.lines, c.tier, fits)
			}
		}
	})
}

func TestLevers(t *testing.T) {
	all := `{"interacting":[{"criterion":"RATE-2","task_criterion":"RATE-12"}],
	"boundaries":[{"criterion":"RATE-12","value":"30","inside":"30","outside":"31","violation":"RATE-12/3"}],
	"shared_helper":{"function":"parcels.common:_band"},"revision":{"criterion":"RATE-4"},"size":true}`

	t.Run("each lever is declared by its member, and a max task declares all five [verifies SP-2918~1] [verifies SP-2924~1]", func(t *testing.T) {
		levers, err := task.ParseLevers([]byte(all))
		if err != nil {
			t.Fatal(err)
		}
		if len(levers.Declared()) != 5 || len(levers.Problems(task.TierMax)) != 0 {
			t.Fatalf("declared %v, problems %v", levers.Declared(), levers.Problems(task.TierMax))
		}
		levers.Revision = nil
		if len(levers.Problems(task.TierMax)) == 0 {
			t.Fatal("a max task without a revision was accepted")
		}
		if len(levers.Problems(task.TierHard)) != 0 {
			t.Fatalf("a hard task with four levers: %v", levers.Problems(task.TierHard))
		}
	})

	t.Run("a hard task declares at least three levers [verifies SP-2918~1]", func(t *testing.T) {
		levers, err := task.ParseLevers([]byte(`{"interacting":[],"size":true}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(levers.Declared()) != 2 || len(levers.Problems(task.TierHard)) == 0 {
			t.Fatalf("declared %v", levers.Declared())
		}
	})

	t.Run("an unknown member, a boundary violation of another criterion and a malformed helper are refused [verifies SP-2918~1]", func(t *testing.T) {
		if _, err := task.ParseLevers([]byte(`{"depth":3}`)); !errors.Is(err, task.ErrMalformedLevers) {
			t.Fatalf("got %v", err)
		}
		levers, err := task.ParseLevers([]byte(strings.Replace(strings.Replace(all, `"RATE-12/3"`, `"RATE-2/3"`, 1), "parcels.common:_band", "_band", 1)))
		if err != nil {
			t.Fatal(err)
		}
		if problems := levers.Problems(task.TierMax); len(problems) != 2 {
			t.Fatalf("problems %v", problems)
		}
	})
}

func TestConfig(t *testing.T) {
	base := `"arms":["control","treatment"],"repetitions":8,"seed":7,"model":"claude-sonnet-5-5",
	"caps":{"assistant_messages":1500,"seconds":7200,"budget_usd":"40.00"},"allowed_tools":["Skill"]`

	t.Run("loop-2.json holds a draw seed and no task before construction [verifies SP-2927~1]", func(t *testing.T) {
		parsed, err := task.ParseConfig([]byte(`{"tasks":[],` + base + `,"draw_seed":20261101}`))
		if err != nil || parsed.DrawSeed != 20261101 {
			t.Fatalf("%+v %v", parsed, err)
		}
		if _, err := task.ParseConfig([]byte(`{"tasks":[],` + base + `}`)); !errors.Is(err, task.ErrConfig) {
			t.Fatalf("no seed: %v", err)
		}
		if _, err := task.ParseConfig([]byte(`{"tasks":[],` + base + `,"draw_seed":1,"tier":"max"}`)); !errors.Is(err, task.ErrConfig) {
			t.Fatalf("unknown member: %v", err)
		}
	})

	t.Run("once it lists a task, loop-1's rules validate the members it shares [verifies SP-2927~1]", func(t *testing.T) {
		listed := `{"tasks":[{"name":"a","fixture":"f","prompt":"p","operations":{"q":{"python":"m:q"}}}],`
		if _, err := task.ParseConfig([]byte(listed + base + `,"draw_seed":1}`)); err != nil {
			t.Fatal(err)
		}
		broken := strings.Replace(base, `"40.00"`, `"forty"`, 1)
		if _, err := task.ParseConfig([]byte(listed + broken + `,"draw_seed":1}`)); !errors.Is(err, task.ErrConfig) {
			t.Fatalf("got %v", err)
		}
	})
}
