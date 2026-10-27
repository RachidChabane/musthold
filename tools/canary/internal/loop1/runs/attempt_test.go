package runs_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func joined(argv []string) string { return strings.Join(argv, " ") }

// agentEdit is what the session changes: the source, a spec it adds, the gate's
// configuration, and a git-ignored cache.
func agentEdit(project string) error {
	return writeAll(project, map[string]string{
		"boxoffice/prices.py":                      "def ticket_price(*, age):\n    return 0 if age < 3 else 12\n",
		"specs/extra.md":                           "# Extra\n",
		"shallnot.yaml":                            "version: 1\nproofs:\n  require: []\n",
		"boxoffice/__pycache__/prices.cpython.pyc": "cache",
	})
}

func (w *world) runOnce(t *testing.T) (runs.Result, string) {
	t.Helper()
	configured := w.first()
	expect, _ := w.inputs.Task(configured.Task.Name)
	dir := study.Layout{Root: w.work}.RunDir(configured.Name(), 1)
	result, err := w.runner.Attempt(context.Background(), runs.Plan{Options: w.options, Inputs: w.inputs, ImageID: testImage, Source: credential{reads: w.reads}},
		runs.Spec{Run: configured, Dir: dir, Attempt: 1, Expect: &expect, Egress: container.HostEndpoints(w.inputs.EgressAllowlist)})
	if err != nil {
		t.Fatal(err)
	}
	return result, dir
}

func TestCompletedAttempt(t *testing.T) {
	w := newWorld(t)
	w.session.edit, w.session.stream = agentEdit, recordedStream(t)
	result, dir := w.runOnce(t)
	records := filepath.Join(dir, plan.RecordsDir)
	run := result.Run
	spec := w.session.specs[0]

	t.Run("run.json describes the attempt with its commits, outcome and models [verifies SP-2523~1]", func(t *testing.T) {
		var written study.Run
		if err := json.Unmarshal([]byte(read(t, filepath.Join(records, record.MetaFile))), &written); err != nil {
			t.Fatal(err)
		}
		expect, _ := w.inputs.Task(w.first().Task.Name)
		if written.Outcome != study.OutcomeCompleted || written.Attempt != 1 || written.BaseCommit != expect.BaseCommit || written.EquippedCommit == "" {
			t.Errorf("run.json: outcome %s (%s), attempt %d, base %s, equipped %s", written.Outcome, written.Reason, written.Attempt, written.BaseCommit, written.EquippedCommit)
		}
		if written.SessionModel != testModel || !slices.Equal(written.UsageModels, []string{testModel}) || written.Model != testModel || written.Prompt == "" {
			t.Errorf("models: session %s, usage %v", written.SessionModel, written.UsageModels)
		}
		if written.Versions["python3"] != "Python 3.13.14" || written.Versions[runs.ShallnotVersionKey] != "shallnot 0.5.1" || written.ClaudeSHA256 != strings.Repeat("c", 64) {
			t.Errorf("versions %v, CLI %s", written.Versions, written.ClaudeSHA256)
		}
		if written.StartedAt.IsZero() || !written.EndedAt.After(written.StartedAt) || written.EventCap != w.config.Caps.AssistantMessages || len(written.Argv) == 0 {
			t.Errorf("times %v %v, caps %d", written.StartedAt, written.EndedAt, written.EventCap)
		}
		for _, name := range []string{record.TranscriptMarkdown, record.TranscriptHTML, record.StepsFile, record.StreamFile, record.HooksFile} {
			if name == record.HooksFile {
				continue
			}
			if _, err := os.Stat(filepath.Join(records, name)); err != nil {
				t.Error(err)
			}
		}
	})

	t.Run("the equipped tree matches the one prepared in another directory [verifies SP-2517~1]", func(t *testing.T) {
		expect, _ := w.inputs.Task(w.first().Task.Name)
		if run.EquippedTree != expect.EquippedTree[string(w.first().Arm)] || run.EquippedTree == "" {
			t.Errorf("tree %s, frozen %v", run.EquippedTree, expect.EquippedTree)
		}
	})

	t.Run("the hashes cover every file of the project, git-ignored ones included and .git left out, before and after [verifies SP-2519~1]", func(t *testing.T) {
		var before, after map[string]string
		_ = json.Unmarshal([]byte(read(t, filepath.Join(records, study.HashesBeforeFile))), &before)
		_ = json.Unmarshal([]byte(read(t, filepath.Join(records, study.HashesAfterFile))), &after)
		if before["boxoffice/prices.py"] == "" || before["boxoffice/prices.py"] == after["boxoffice/prices.py"] || after["boxoffice/__pycache__/prices.cpython.pyc"] == "" {
			t.Errorf("before %v\nafter %v", before, after)
		}
		for path := range after {
			if strings.HasPrefix(path, ".git/") {
				t.Errorf("hashed %s", path)
			}
		}
		if before["shallnot.yaml"] == after["shallnot.yaml"] {
			t.Error("the session's edit of shallnot.yaml is not seen")
		}
	})

	t.Run("the acceptance gate reads the equipped shallnot.yaml, and the project keeps the session's own [verifies SP-2520~1]", func(t *testing.T) {
		var report struct{ Config string }
		_ = json.Unmarshal([]byte(read(t, filepath.Join(records, record.FinalGateFile))), &report)
		kinds, _ := w.first().Arm.Require()
		if !strings.Contains(report.Config, "require: ["+strings.Join(kinds, ", ")+"]") || !strings.Contains(report.Config, "trust: local") {
			t.Errorf("the gate read:\n%s", report.Config)
		}
		project := filepath.Join(dir, plan.ProjectDir)
		if got := read(t, filepath.Join(project, "shallnot.yaml")); got != "version: 1\nproofs:\n  require: []\n" {
			t.Errorf("the project's shallnot.yaml is %q", got)
		}
		if read(t, filepath.Join(project, "specs", "extra.md")) != "# Extra\n" {
			t.Error("the spec the session added is gone")
		}
		if !strings.Contains(read(t, filepath.Join(records, record.ChangeFile)), "specs/extra.md") {
			t.Error("change.diff lacks the added spec")
		}
	})

	t.Run("container.json holds the image's id, the exit code and the memory kill, read with a format that selects them alone [verifies SP-2521~1]", func(t *testing.T) {
		var state study.ContainerState
		_ = json.Unmarshal([]byte(read(t, filepath.Join(records, study.ContainerFile))), &state)
		if state != (study.ContainerState{ImageDigest: testImage, ExitCode: 0, OOMKilled: false}) {
			t.Errorf("container.json %+v", state)
		}
		for _, argv := range w.engine.argvs() {
			if len(argv) > 1 && argv[1] == "inspect" && !slices.Equal(argv[2:4], []string{"--format", "{{.State.ExitCode}} {{.State.OOMKilled}}"}) {
				t.Errorf("an inspect reads more: %v", argv)
			}
		}
	})

	t.Run("the session settings allow the configured rules and deny the web tools [verifies SP-2512~1]", func(t *testing.T) {
		var settings map[string]map[string][]string
		_ = json.Unmarshal([]byte(read(t, filepath.Join(dir, plan.SessionSettings))), &settings)
		if !slices.Equal(settings["permissions"]["allow"], w.config.AllowedTools) || !slices.Equal(settings["permissions"]["deny"], []string{"WebFetch", "WebSearch"}) {
			t.Errorf("settings %v", settings)
		}
	})

	t.Run("the session runs the image's CLI, every container is bounded to 3 GiB and 1024 processes with a HOME of the run's own, and the loopback skips the proxy [verifies SP-2514~1]", func(t *testing.T) {
		session := joined(spec.Argv)
		for _, wanted := range []string{"entry -- " + testClaude + " -p", "--memory 3g --memory-swap 3g --pids-limit 1024", "HOME=" + filepath.Join(dir, plan.HomeDir), "NO_PROXY=localhost,127.0.0.1"} {
			if !strings.Contains(session, wanted) {
				t.Errorf("the session lacks %q", wanted)
			}
		}
		for _, argv := range w.engine.argvs() {
			if line := joined(argv); slices.Contains(argv, "--rm") && (!strings.Contains(line, "--memory 3g") || !strings.Contains(line, "--pids-limit 1024") || !strings.Contains(line, "HOME="+filepath.Join(dir, plan.HomeDir))) {
				t.Errorf("a step is not bounded: %s", line)
			}
		}
		entries, err := os.ReadDir(filepath.Join(dir, plan.HomeDir))
		if err != nil || len(entries) != 0 {
			t.Errorf("the run's HOME: %v, %v", entries, err)
		}
	})

	t.Run("the proxy forwards the frozen allowlist's hosts on port 443 alone, and its log is among the records [verifies SP-2515~1]", func(t *testing.T) {
		var proxy []string
		for _, argv := range w.engine.argvs() {
			if slices.Contains(argv, "--detach") {
				proxy = argv
			}
		}
		allows := []string{}
		for index, word := range proxy {
			if word == "--allow" {
				allows = append(allows, proxy[index+1])
			}
		}
		if !slices.Equal(allows, []string{allowedHost + ":443"}) {
			t.Errorf("the proxy allows %v", allows)
		}
		if !strings.Contains(read(t, filepath.Join(records, record.EgressFile)), allowedHost) {
			t.Error("egress.log is not in the records")
		}
	})

	t.Run("Shallnot's shim records each end-of-turn call's report and tree [verifies SP-2518~1]", func(t *testing.T) {
		shim := read(t, filepath.Join(dir, plan.ShimDir, plan.ShallnotBinary))
		for _, wanted := range []string{"check --json-out", filepath.Join(records, plan.StopsRecordDir), "GIT_INDEX_FILE=", "GIT_OBJECT_DIRECTORY=" + filepath.Join(dir, plan.StopObjectsDir)} {
			if !strings.Contains(shim, wanted) {
				t.Errorf("the shim lacks %q:\n%s", wanted, shim)
			}
		}
	})

	t.Run("every record is scanned and none held the token [verifies SP-2522~1]", func(t *testing.T) {
		var scan study.TokenScan
		_ = json.Unmarshal([]byte(read(t, filepath.Join(records, study.TokenScanFile))), &scan)
		if !slices.Contains(scan.Scanned, record.MetaFile) || !slices.Contains(scan.Scanned, record.StderrFile) || len(scan.Replaced) != 0 || result.TokenMatches != 0 {
			t.Errorf("scan %+v", scan)
		}
	})
}

func TestEquippedArms(t *testing.T) {
	w := newWorld(t)
	equipper := runs.Equipper{Options: w.options, Processes: w.engine, Files: adapters.OSFiles{}, Tree: runs.OSTree{}}
	projects := map[config.Arm]string{}
	for _, arm := range []config.Arm{config.ArmControl, config.ArmTreatment} {
		dir := filepath.Join(tempDir(t), "run")
		if _, err := equipper.Equip(context.Background(), dir, config.Run{Task: w.config.Tasks[0], Arm: arm, Repetition: 1}); err != nil {
			t.Fatal(err)
		}
		projects[arm] = filepath.Join(dir, plan.ProjectDir)
	}

	t.Run("both arms are equipped alike and differ only in proofs.require of shallnot.yaml [verifies SP-2511~1]", func(t *testing.T) {
		control, treatment := read(t, filepath.Join(projects[config.ArmControl], "shallnot.yaml")), read(t, filepath.Join(projects[config.ArmTreatment], "shallnot.yaml"))
		if !strings.Contains(control, "  require: []\n") || !strings.Contains(treatment, "  require: [discriminating-power, alignment]\n") {
			t.Errorf("control:\n%s\ntreatment:\n%s", control, treatment)
		}
		if strings.Replace(control, "[]", "[discriminating-power, alignment]", 1) != treatment {
			t.Error("the configurations differ beyond proofs.require")
		}
		filesOf := func(project string) []string {
			files, _ := runs.OSTree{}.Files(project, []string{".git"})
			return files
		}
		if !slices.Equal(filesOf(projects[config.ArmControl]), filesOf(projects[config.ArmTreatment])) {
			t.Error("the arms hold different files")
		}
		for _, file := range filesOf(projects[config.ArmControl]) {
			if file == "shallnot.yaml" {
				continue
			}
			a := strings.ReplaceAll(read(t, filepath.Join(projects[config.ArmControl], file)), filepath.Dir(projects[config.ArmControl]), "<run>")
			b := strings.ReplaceAll(read(t, filepath.Join(projects[config.ArmTreatment], file)), filepath.Dir(projects[config.ArmTreatment]), "<run>")
			if a != b {
				t.Errorf("%s differs between the arms", file)
			}
		}
	})
}

func TestTokenExposed(t *testing.T) {
	w := newWorld(t)
	w.session.leak = true
	if err := w.batch().Run(context.Background(), w.invocation()); err != nil {
		t.Fatal(err)
	}
	dir := study.Layout{Root: w.work}.RunDir(w.inputs.Order[0], 1)
	records := filepath.Join(dir, plan.RecordsDir)

	t.Run("a token in a record is replaced, listed in the token scan, logged as token-exposed through the stamper, and the run stays counted [verifies SP-2522~1]", func(t *testing.T) {
		if got := read(t, filepath.Join(records, record.StderrFile)); strings.Contains(got, testToken) || !strings.Contains(got, record.TokenPlaceholder) {
			t.Errorf("stderr.txt = %q", got)
		}
		var scan study.TokenScan
		_ = json.Unmarshal([]byte(read(t, filepath.Join(records, study.TokenScanFile))), &scan)
		if scan.Replaced[record.StderrFile] != 1 {
			t.Errorf("scan %+v", scan)
		}
		var run study.Run
		_ = json.Unmarshal([]byte(read(t, filepath.Join(records, record.MetaFile))), &run)
		if !run.Outcome.Counted() {
			t.Errorf("outcome %s", run.Outcome)
		}
		deviations := study.ParseDeviations(read(t, filepath.Join(w.work, study.DeviationsFile)))
		if !slices.ContainsFunc(deviations, func(d study.Deviation) bool { return d.Kind == study.KindTokenExposed }) || len(w.stamps.lines) == 0 {
			t.Errorf("deviations %+v, stamped %v", deviations, w.stamps.lines)
		}
	})
}

func TestTruthIsolation(t *testing.T) {
	w := newWorld(t)
	repo := tempDir(t)
	const sentinel = "TRUTH-SENTINEL-7f3a"
	task := filepath.Join(repo, "experiments", "loop-1", "tasks", "parcel")
	fixture := filepath.Join(task, "fixture")
	if err := os.CopyFS(fixture, os.DirFS(filepath.Join(w.repo, w.config.Tasks[0].Fixture))); err != nil {
		t.Fatal(err)
	}
	truth := map[string]string{}
	for _, name := range []string{"oracle", "reference", "violations", "refactors"} {
		for _, root := range []string{task, fixture + "-" + name} {
			path := filepath.Join(root, name)
			if err := writeAll(path, map[string]string{"test_truth.py": sentinel + " " + name + "\n"}); err != nil {
				t.Fatal(err)
			}
			truth[name] = path
		}
	}
	if err := writeAll(repo, map[string]string{plan.ClaudePinRelative: testPin + "\n"}); err != nil {
		t.Fatal(err)
	}
	digests := []string{runs.SHA256([]byte(sentinel + " oracle\n"))}
	w.options.RepoRoot = repo
	configured := config.Run{Task: config.Task{Name: "parcel", Fixture: "experiments/loop-1/tasks/parcel/fixture", Prompt: w.config.Tasks[0].Prompt, Operations: w.config.Tasks[0].Operations}, Arm: config.ArmTreatment, Repetition: 1}
	dir := study.Layout{Root: w.work}.RunDir(configured.Name(), 1)
	inputs := w.inputs
	inputs.Tasks = []study.TaskInputs{{Name: "parcel", Truth: map[string]string{"oracle": digests[0]}}}
	if _, err := w.runner.Attempt(context.Background(), runs.Plan{Options: w.options, Inputs: inputs, ImageID: testImage, Source: credential{reads: w.reads}},
		runs.Spec{Run: configured, Dir: dir, Attempt: 1, Egress: container.HostEndpoints(inputs.EgressAllowlist)}); err != nil {
		t.Fatal(err)
	}
	forbidden := append([]string{sentinel, "experiments/loop-1/tasks/parcel/oracle", "fixture-oracle"}, digests...)
	for _, path := range truth {
		forbidden = append(forbidden, path)
	}

	t.Run("no truth file, truth path or truth digest is in the run directory, the session's mounts, its prompt or its settings [verifies SP-2533~1]", func(t *testing.T) {
		files, err := runs.OSTree{}.Files(dir, nil)
		if err != nil || len(files) == 0 {
			t.Fatalf("files %v, %v", files, err)
		}
		for _, file := range files {
			content, err := runs.OSTree{}.Read(filepath.Join(dir, file))
			if err != nil {
				t.Fatal(err)
			}
			for _, word := range forbidden {
				if strings.Contains(string(content), word) || strings.Contains(file, word) {
					t.Errorf("%s holds %q", file, word)
				}
			}
		}
		session := w.session.specs[0].Argv
		var mounts []string
		for index, word := range session {
			if word == "--mount" {
				mounts = append(mounts, session[index+1])
			}
		}
		if !slices.Equal(mounts, []string{"type=bind,source=" + dir + ",target=" + dir, "type=bind,source=" + w.product + ",target=/opt/musthold,readonly"}) {
			t.Errorf("mounts %v", mounts)
		}
		for _, word := range forbidden {
			if strings.Contains(joined(session), word) {
				t.Errorf("the session's command holds %q", word)
			}
		}
	})
}

func TestPreparationFailure(t *testing.T) {
	w := newWorld(t)
	w.engine.failInit = true
	result, dir := w.runOnce(t)

	t.Run("a failed preparation step is a harness error with run.json and the steps written, and no session starts [verifies SP-2523~1]", func(t *testing.T) {
		if result.Run.Outcome != study.OutcomeHarnessError || !strings.Contains(result.Run.Reason, "init") || len(w.session.specs) != 0 {
			t.Errorf("outcome %s (%s), sessions %d", result.Run.Outcome, result.Run.Reason, len(w.session.specs))
		}
		for _, name := range []string{record.MetaFile, record.StepsFile, study.TokenScanFile} {
			if _, err := os.Stat(filepath.Join(dir, plan.RecordsDir, name)); err != nil {
				t.Error(err)
			}
		}
	})

	t.Run("an attempt never reuses a directory [verifies SP-2527~1]", func(t *testing.T) {
		_, err := w.runner.Attempt(context.Background(), runs.Plan{Options: w.options, Inputs: w.inputs}, runs.Spec{Run: w.first(), Dir: dir, Attempt: 1})
		if !errors.Is(err, runs.ErrAttemptExists) {
			t.Errorf("error %v", err)
		}
	})
}

var _ = app.ProbesFile
