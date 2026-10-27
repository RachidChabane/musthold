package freeze_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/product"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func TestCheck(t *testing.T) {
	t.Run("a study frozen as PREREG §13 freezes it passes every condition [verifies SP-2624~1]", func(t *testing.T) {
		built := studyFor(t, scenario{runs: []freeze.RunStart{{Name: "alpha-traceability-1-1", Start: t0.Add(5 * time.Hour)}}})
		if failures := built.check.Check(t.Context()); len(failures) != 0 {
			t.Fatalf("failures: %v", failures)
		}
	})

	t.Run("a missing amendment tag and a commit file that is not the peeled tag are both refused [verifies SP-2624~1]", func(t *testing.T) {
		built := studyFor(t, scenario{skipTag: freeze.AmendmentTag, commitFile: map[string]string{freeze.PreregTag: "deadbeef\n"}})
		failures := built.check.Check(t.Context())
		conditions := failureConditions(failures)
		if !conditions[freeze.CondTags] || !conditions[freeze.CondCommitFiles] || !strings.Contains(joined(failures), freeze.AmendmentTag) || !strings.Contains(joined(failures), "prereg-commit.txt") {
			t.Errorf("failures %v", failures)
		}
	})

	t.Run("an imprint of another content is refused [verifies SP-2624~1]", func(t *testing.T) {
		failures := studyFor(t, scenario{badImprint: map[string]bool{freeze.PreregTag: true}}).check.Check(t.Context())
		if !failureConditions(failures)[freeze.CondImprints] || !strings.Contains(joined(failures), freeze.PreregTag) {
			t.Errorf("failures %v", failures)
		}
	})
}

func joined(failures []freeze.Failure) string {
	var text []string
	for _, failure := range failures {
		text = append(text, failure.String())
	}
	return strings.Join(text, "\n")
}

func TestCheckOrder(t *testing.T) {
	cases := []struct {
		name     string
		scenario scenario
		detail   string
	}{
		{"an amendment stamped after a construction call", scenario{genTimes: map[string]time.Time{freeze.AmendmentTag: t0.Add(150 * time.Minute)}}, "does not precede construction call 1 start"},
		{"a pre-registration stamped after a canary session", scenario{genTimes: map[string]time.Time{freeze.PreregTag: t0.Add(200 * time.Minute)}}, "canary session c1.jsonl"},
		{"inputs stamped before a canary session", scenario{genTimes: map[string]time.Time{freeze.InputsTag: t0.Add(170 * time.Minute)}}, "does not precede " + freeze.InputsTag},
		{"a run that started before the inputs were stamped", scenario{runs: []freeze.RunStart{{Name: "early", Start: t0.Add(time.Hour)}}}, "start of early"},
	}
	for _, c := range cases {
		t.Run(c.name+" breaks the order [verifies SP-2625~1]", func(t *testing.T) {
			failures := studyFor(t, c.scenario).check.Check(t.Context())
			if !failureConditions(failures)[freeze.CondOrder] || !strings.Contains(joined(failures), c.detail) {
				t.Errorf("want %q, got %v", c.detail, failures)
			}
		})
	}

	t.Run("OrderFailures requires the construction calls and canary sessions to exist and to carry a time [verifies SP-2625~1]", func(t *testing.T) {
		stamped := freeze.Timeline{Prereg: t0, Amendment: t0.Add(time.Minute), Inputs: t0.Add(time.Hour)}
		if got := freeze.OrderFailures(stamped); len(got) != 2 {
			t.Errorf("no call and no session: %v", got)
		}
		stamped.Calls = []records.Call{{CodexVersion: "c"}}
		stamped.Canary = []study.CanaryRecord{{Name: "c1", Start: t0.Add(2 * time.Minute)}}
		if got := freeze.OrderFailures(stamped); len(got) != 1 || !strings.Contains(got[0], "has no start") {
			t.Errorf("a call without a start: %v", got)
		}
		stamped.Calls[0].Start = t0.Add(3 * time.Minute)
		if got := freeze.OrderFailures(stamped); len(got) != 0 {
			t.Errorf("an ordered timeline: %v", got)
		}
		stamped.Calls[0].End = t0.Add(2 * time.Hour)
		if got := freeze.OrderFailures(stamped); len(got) != 1 {
			t.Errorf("a call that ends after the inputs were stamped: %v", got)
		}
		if got := freeze.OrderFailures(freeze.Timeline{}); len(got) < 3 {
			t.Errorf("unknown generation times: %v", got)
		}
	})
}

func TestCheckSums(t *testing.T) {
	flip := func(text string) string {
		return strings.Replace(text, text[:1], map[bool]string{true: "0", false: "1"}[text[0] != '0'], 1)
	}
	dropLine := func(text string) string { return text[strings.Index(text, "\n")+1:] }
	addLine := func(text string) string { return text + strings.Repeat("a", 64) + "  experiments/loop-1/ghost\n" }
	cases := []struct {
		name     string
		scenario scenario
		details  []string
	}{
		{"a pre-registration digest that differs and an inputs file the sums omit", scenario{preregSums: flip, inputsSums: dropLine},
			[]string{"SHA256SUMS lists", "inputs-SHA256SUMS does not list"}},
		{"a pre-registration file the sums omit and an inputs digest that differs", scenario{preregSums: dropLine, inputsSums: flip},
			[]string{"SHA256SUMS does not list", "inputs-SHA256SUMS lists"}},
		{"a sum of a file the tag lacks", scenario{preregSums: addLine}, []string{"does not hold"}},
	}
	for _, c := range cases {
		t.Run(c.name+" are refused [verifies SP-2626~1]", func(t *testing.T) {
			failures := studyFor(t, c.scenario).check.Check(t.Context())
			if !failureConditions(failures)[freeze.CondSums] {
				t.Fatalf("failures %v", failures)
			}
			for _, detail := range c.details {
				if !strings.Contains(joined(failures), detail) {
					t.Errorf("no failure names %q: %v", detail, failures)
				}
			}
		})
	}
}

func TestCheckPinsAndPolicy(t *testing.T) {
	t.Run("the stale Shallnot pin of the frozen text, a Claude Code version and toolchains that differ are refused [verifies SP-2627~1]", func(t *testing.T) {
		built := studyFor(t, scenario{tamperInputs: func(in *study.Inputs) {
			in.Shallnot.Pin = "5e4783ea07de79655769cc3701048d12f09d716d"
			in.ClaudeCode.Version = "2.1.296"
			in.Toolchains[freeze.ToolchainPython] = "3.13.13"
			in.Toolchains[freeze.ToolchainPytest] = "9.0.0"
			in.Toolchains[freeze.ToolchainColimaC] = "4"
			in.Policy["completeness"]["max_hunks"] = "51"
		}, policyEdit: map[string]string{"discpower.k_op": "399", "alignment.min_isolation": "other"}})
		details := joined(built.check.Check(t.Context()))
		for _, want := range []string{
			"the Shallnot pin", "Claude Code", "toolchain python", "toolchain pytest", "toolchain colima_cpus",
			"completeness.max_hunks", "discpower.k_op", "alignment.min_isolation", "the source at",
		} {
			if !strings.Contains(details, want) {
				t.Errorf("no failure names %q: %s", want, details)
			}
		}
	})
	t.Run("the amended Shallnot pin and the table of module defaults are accepted [verifies SP-2627~1]", func(t *testing.T) {
		if failures := studyFor(t, scenario{}).check.Check(t.Context()); len(failures) != 0 {
			t.Errorf("failures %v", failures)
		}
	})
}

func TestCheckInstructions(t *testing.T) {
	t.Run("arms that differ in shallnot.yaml alone pass [verifies SP-2627~1]", func(t *testing.T) {
		if failureConditions(studyFor(t, scenario{}).check.Check(t.Context()))[freeze.CondInstructions] {
			t.Error("the default preparer differs in shallnot.yaml alone")
		}
	})
	cases := map[string]scenario{
		"a file other than shallnot.yaml": {armFiles: func(arm config.Arm) map[string]string {
			return map[string]string{"shallnot.yaml": "1", "AGENTS.md": strings.Repeat(string(arm[0]), 64)}
		}},
		"a file only one arm holds": {armFiles: func(arm config.Arm) map[string]string {
			files := map[string]string{"shallnot.yaml": "1"}
			if arm == config.ArmTraceability {
				files["extra.md"] = strings.Repeat("e", 64)
			}
			return files
		}},
	}
	for name, sc := range cases {
		t.Run(name+" is refused [verifies SP-2627~1]", func(t *testing.T) {
			built := studyFor(t, sc)
			if !failureConditions(built.check.Check(t.Context()))[freeze.CondInstructions] {
				t.Error("the difference passed")
			}
		})
	}
	t.Run("a prompt, settings or plugin tree that differs is named [verifies SP-2627~1]", func(t *testing.T) {
		task := study.TaskInputs{Name: "t", Instructions: map[string]study.ArmInstructions{
			"a": {Prompt: "p", SessionSettings: "s", Plugins: map[string]string{"x": "1"}},
			"b": {Prompt: "q", SessionSettings: "t", Plugins: map[string]string{"x": "2"}},
		}}
		if got := freeze.InstructionFailures(task); len(got) != 3 {
			t.Errorf("failures %v", got)
		}
		if got := freeze.InstructionFailures(study.TaskInputs{Name: "single"}); len(got) != 1 {
			t.Errorf("a task with no arms: %v", got)
		}
	})
}

func TestCheckProduct(t *testing.T) {
	t.Run("a binary that changed since resolve is refused [verifies SP-2628~1]", func(t *testing.T) {
		built := studyFor(t, scenario{tamperProduct: func(dir string) { write(t, dir, "bin/shallnot", "another build") }})
		failures := built.check.Check(t.Context())
		if !failureConditions(failures)[freeze.CondBinaries] || !strings.Contains(joined(failures), "shallnot") {
			t.Errorf("failures %v", failures)
		}
	})
	t.Run("a plugin tree that changed since resolve is refused [verifies SP-2628~1]", func(t *testing.T) {
		built := studyFor(t, scenario{tamperProduct: func(dir string) {
			write(t, dir, "plugins/"+product.PluginPaths()[0]+"/extra.json", "{}")
		}})
		if !failureConditions(built.check.Check(t.Context()))[freeze.CondBinaries] {
			t.Error("the changed tree passed")
		}
	})
	t.Run("an image the engine does not hold under the recorded id, and a manifest of another image, are refused [verifies SP-2628~1]", func(t *testing.T) {
		built := studyFor(t, scenario{engineImage: "sha256:" + strings.Repeat("9", 64)})
		if !failureConditions(built.check.Check(t.Context()))[freeze.CondImage] {
			t.Error("another image passed")
		}
		built = studyFor(t, scenario{tamperProduct: func(dir string) {
			data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			write(t, dir, "manifest.json", strings.Replace(string(data), strings.Repeat("1", 64), strings.Repeat("7", 64), 1))
		}})
		if !strings.Contains(joined(built.check.Check(t.Context())), "image build") {
			t.Error("another build image passed")
		}
	})
}

func TestCheckInputsCommit(t *testing.T) {
	t.Run("commits that the tags do not support are refused [verifies SP-2624~1]", func(t *testing.T) {
		built := studyFor(t, scenario{tamperInputs: func(in *study.Inputs) {
			in.Commits.Inputs = strings.Repeat("1", 40)
			in.Commits.Prereg = strings.Repeat("2", 40)
		}})
		details := joined(built.check.Check(t.Context()))
		for _, want := range []string{"the parent of " + freeze.InputsTag, "commits.prereg"} {
			if !strings.Contains(details, want) {
				t.Errorf("no failure names %q: %s", want, details)
			}
		}
	})
}

func TestCheckReportsEveryFailure(t *testing.T) {
	t.Run("independent failures are all reported, each under its condition [verifies SP-2629~1]", func(t *testing.T) {
		built := studyFor(t, scenario{
			skipTag:           freeze.AmendmentTag,
			badImprint:        map[string]bool{freeze.PreregTag: true},
			preregSums:        func(text string) string { return text[strings.Index(text, "\n")+1:] },
			policyEdit:        map[string]string{"discpower.k_op": "399"},
			tamperProduct:     func(dir string) { write(t, dir, "bin/shallnot", "another build") },
			tamperInputs:      func(in *study.Inputs) { in.ClaudeCode.Version = "2.1.296" },
			commitFile:        map[string]string{freeze.InputsTag: "deadbeef\n"},
			engineImage:       "sha256:" + strings.Repeat("9", 64),
			genTimes:          map[string]time.Time{freeze.InputsTag: t0.Add(-time.Hour)},
			constructionStart: t0.Add(2 * time.Hour),
		})
		conditions := failureConditions(built.check.Check(t.Context()))
		for _, want := range []string{freeze.CondTags, freeze.CondImprints, freeze.CondSums, freeze.CondPolicy, freeze.CondBinaries, freeze.CondPins, freeze.CondCommitFiles, freeze.CondImage, freeze.CondOrder} {
			if !conditions[want] {
				t.Errorf("%s is not reported among %v", want, conditions)
			}
		}
	})
}

func TestCheckMain(t *testing.T) {
	t.Run("the verb names each failed condition on its error stream and exits 1, and exits 2 on a usage error [verifies SP-2629~1]", func(t *testing.T) {
		var stdout, stderr strings.Builder
		if code := freeze.CheckMain(t.Context(), []string{"--nonsense"}, &stdout, &stderr); code != freeze.ExitUsage {
			t.Errorf("usage: code %d", code)
		}
		stderr.Reset()
		empty := t.TempDir()
		code := freeze.CheckMain(t.Context(), []string{"--repo", empty, "--work", empty, "--product", empty}, &stdout, &stderr)
		if code != freeze.ExitFailed || !strings.Contains(stderr.String(), "FAIL "+freeze.CondTags) || stdout.Len() != 0 {
			t.Errorf("code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
		}
	})
}
