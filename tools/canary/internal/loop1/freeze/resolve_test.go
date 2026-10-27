package freeze_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/deviation"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/product"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func resolved(t *testing.T, built studyRepo) study.Inputs {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(built.root, filepath.FromSlash(freeze.InputsFile)))
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := study.ParseInputs(data)
	if err != nil {
		t.Fatal(err)
	}
	return inputs
}

func TestResolve(t *testing.T) {
	built := shared(t)
	inputs := resolved(t, built)
	repo := freeze.Git{Dir: built.root, Exec: freeze.OSExec{}}

	t.Run("the commits are the pre-registration's, HEAD at resolve time, and the product commits between [verifies SP-2616~1]", func(t *testing.T) {
		prereg, err := repo.PeelTag(t.Context(), freeze.PreregTag)
		if err != nil {
			t.Fatal(err)
		}
		tagged, err := repo.PeelTag(t.Context(), freeze.InputsTag)
		if err != nil {
			t.Fatal(err)
		}
		parent, err := repo.Parent(t.Context(), tagged)
		if err != nil {
			t.Fatal(err)
		}
		if inputs.Commits.Prereg != prereg || inputs.Commits.Inputs != parent {
			t.Errorf("commits %+v", inputs.Commits)
		}
		if !slices.Equal(inputs.Commits.Between, []string{parent}) {
			t.Errorf("between %v: only the tooling commit touches a product directory", inputs.Commits.Between)
		}
	})

	t.Run("the binaries, the plugin trees, the images and the toolchains come from the product, the engine and the image [verifies SP-2617~1]", func(t *testing.T) {
		for _, name := range product.RequiredBinaries() {
			if len(inputs.Binaries[name]) != 64 {
				t.Errorf("binary %s: %q", name, inputs.Binaries[name])
			}
		}
		for _, plugin := range product.PluginPaths() {
			if len(inputs.Binaries[freeze.PluginKeyPrefix+plugin]) != 64 {
				t.Errorf("plugin %s", plugin)
			}
		}
		if !strings.HasPrefix(inputs.Images.Build, "golang:1.26.4") || inputs.Images.Built != "sha256:"+imageDigest || inputs.Images.Proxy != inputs.Images.Built {
			t.Errorf("images %+v", inputs.Images)
		}
		want := map[string]string{
			freeze.ToolchainGo: "1.26.4", freeze.ToolchainPython: "3.13.14", freeze.ToolchainPytest: "9.1.1", freeze.ToolchainUV: "0.11.23",
			freeze.ToolchainNode: "24.1.0", freeze.ToolchainGit: "2.39.5", freeze.ToolchainDocker: "27.5.1", freeze.ToolchainColima: "0.8.1",
			freeze.ToolchainColimaC: "2", freeze.ToolchainColimaM: "4294967296", freeze.ToolchainColimaD: "32212254720",
			freeze.ToolchainHostOS: "Darwin", freeze.ToolchainHostVer: "25.6.0", freeze.ToolchainHostArch: "arm64",
		}
		for key, value := range want {
			if inputs.Toolchains[key] != value {
				t.Errorf("toolchain %s = %q, want %q", key, inputs.Toolchains[key], value)
			}
		}
	})

	t.Run("Shallnot is recorded with its pin, its version and the first public release that holds the pin [verifies SP-2618~1]", func(t *testing.T) {
		want := study.Shallnot{Pin: freeze.ExpectedShallnotPin, Version: "shallnot v0.5.1", Release: "v0.5.1"}
		if inputs.Shallnot != want {
			t.Errorf("shallnot %+v", inputs.Shallnot)
		}
	})

	t.Run("Claude Code is recorded with its version and the SHA-256 of the binary the symlink names [verifies SP-2619~1]", func(t *testing.T) {
		if inputs.ClaudeCode != (study.ClaudeCode{Version: "2.1.295", SHA256: claudeDigest}) {
			t.Errorf("claude %+v", inputs.ClaudeCode)
		}
	})

	t.Run("the Codex versions, the construction records and the canary's findings are copied [verifies SP-2620~1]", func(t *testing.T) {
		if !slices.Equal(inputs.Codex, []string{"codex-cli 0.9.0"}) {
			t.Errorf("codex %v", inputs.Codex)
		}
		calls, err := freeze.ConstructionCalls(inputs.Construction)
		if err != nil || len(calls) != 2 || calls[0].Start.IsZero() {
			t.Errorf("calls %+v, %v", calls, err)
		}
		if !slices.Equal(inputs.CLIAuxiliaryModels, []string{"claude-haiku-4-5"}) || !slices.Equal(inputs.EgressAllowlist, []string{"api.anthropic.com"}) ||
			len(inputs.EgressObserved) != 2 || !slices.Equal(inputs.QuotaPatterns, []string{"usage limit reached"}) || len(inputs.Canary) != 1 {
			t.Errorf("canary %+v", inputs)
		}
	})

	t.Run("the policy is the module defaults the source declares at the product commit [verifies SP-2621~1]", func(t *testing.T) {
		if got := freeze.PolicyFailures("inputs", inputs.Policy, freeze.ExpectedPolicy); len(got) != 0 {
			t.Errorf("policy differs: %v", got)
		}
	})

	t.Run("each task records its trees, criterion, prompt, start commit, equipped trees, instructions per arm and truth digests [verifies SP-2622~1]", func(t *testing.T) {
		task, ok := inputs.Task("alpha")
		if !ok {
			t.Fatal("no task alpha")
		}
		fixture, err := product.TreeDigest(os.DirFS(filepath.Join(built.root, "fixtures/alpha")))
		if err != nil {
			t.Fatal(err)
		}
		if task.FixtureTree != fixture || task.Criterion != "ALP-4" || task.PromptSHA256 != deviation.Imprint([]byte("Do alpha.")) || task.BaseCommit != strings.Repeat("b", 40) {
			t.Errorf("task %+v", task)
		}
		if task.EquippedTree["traceability"] == "" || task.EquippedTree["four-proofs"] == "" || task.EquippedTree["traceability"] == task.EquippedTree["four-proofs"] {
			t.Errorf("equipped %v", task.EquippedTree)
		}
		for arm, instructions := range task.Instructions {
			if instructions.Prompt != task.PromptSHA256 || instructions.SessionSettings == "" || len(instructions.Plugins) != len(product.PluginPaths()) || instructions.Files["AGENTS.md"] == "" {
				t.Errorf("arm %s: %+v", arm, instructions)
			}
		}
		for _, key := range []string{freeze.TruthOracle, freeze.TruthReference, freeze.TruthViolations, freeze.TruthRefactors} {
			if len(task.Truth[key]) != 64 {
				t.Errorf("truth %s: %q", key, task.Truth[key])
			}
		}
	})

	t.Run("the order is the configuration's seeded order [verifies SP-2623~1]", func(t *testing.T) {
		configured, err := config.Parse([]byte(configJSON))
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		for _, run := range configured.Runs() {
			want = append(want, run.Name())
		}
		if len(want) != 4 || !slices.Equal(inputs.Order, want) {
			t.Errorf("order %v, want %v", inputs.Order, want)
		}
	})

	t.Run("the sums list every file of the task, the fixture, the configuration, the image and inputs.json, in shasum's format [verifies SP-2623~1]", func(t *testing.T) {
		text, err := os.ReadFile(filepath.Join(built.root, filepath.FromSlash(freeze.InputsSumsFile)))
		if err != nil {
			t.Fatal(err)
		}
		sums, err := freeze.ParseSums(string(text))
		if err != nil {
			t.Fatal(err)
		}
		listed := map[string]string{}
		for _, sum := range sums {
			listed[sum.Path] = sum.Digest
		}
		for _, file := range []string{
			"experiments/loop-1/loop-1.json", "experiments/loop-1/tasks/alpha/task.json", "experiments/loop-1/tasks/alpha/oracle/test_a.py",
			"experiments/loop-1/tasks/alpha/violations/a/1/x.py", "experiments/loop-1/image/Dockerfile", "experiments/loop-1/image/requirements.lock",
			"fixtures/alpha/alpha.py", freeze.InputsFile,
		} {
			if listed[file] == "" {
				t.Errorf("%s is not listed", file)
			}
		}
		document, _ := os.ReadFile(filepath.Join(built.root, filepath.FromSlash(freeze.InputsFile)))
		if listed[freeze.InputsFile] != deviation.Imprint(document) || string(text) != freeze.FormatSums(sums) {
			t.Error("the sums are not the SHA-256 of the written files in shasum's format")
		}
		if listed["experiments/loop-1/PREREG.md"] != "" {
			t.Error("the protocol is listed")
		}
	})
}

func TestResolveRefusals(t *testing.T) {
	t.Run("a tracked file that differs from HEAD is refused [verifies SP-2616~1]", func(t *testing.T) {
		built := newStudy(t, scenario{})
		write(t, built.root, "fixtures/alpha/alpha.py", "changed")
		_, err := resolver(t, built.root, built.productDir, built.work, scenario{}).Resolve(t.Context())
		if err == nil || !strings.Contains(err.Error(), "differs from HEAD") {
			t.Errorf("error %v", err)
		}
	})
	t.Run("a product built from another commit than HEAD is refused [verifies SP-2617~1]", func(t *testing.T) {
		built := newStudy(t, scenario{})
		_, err := resolver(t, built.root, built.productDir, built.work, scenario{}).Resolve(t.Context())
		if err == nil || !strings.Contains(err.Error(), "does not match the repository") {
			t.Errorf("error %v", err)
		}
	})
	t.Run("a Claude Code version other than the protocol's is refused [verifies SP-2619~1]", func(t *testing.T) {
		built := newStudy(t, scenario{claudeOutput: "2.1.296 (Claude Code)", resolveFails: true})
		if built.resolveErr == nil || !strings.Contains(built.resolveErr.Error(), "2.1.296") {
			t.Errorf("error %v", built.resolveErr)
		}
	})
}
