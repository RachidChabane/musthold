// Package fixturegate checks the demonstration's fixture with the real binaries
// and no model: equipped the way the runner equips it, the project's gate blocks
// once the task is done with its weak test left as it is, and passes once that
// test is strengthened by hand. A simulated Claude Code serves every ticket
// through each module's real hooks and executor server, with answers replayed
// from recordings that the honest host of the project wrote.
package fixturegate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest/claudecode"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/boxoffice"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/modules"
)

// update rewrites the recorded answers from the honest host instead of replaying them.
var update = flag.Bool("update", false, "record the honest host's answers into testdata/answers instead of replaying them")

// keep leaves the run directory in place and names it, to read what the modules wrote.
var keep = flag.Bool("keep", false, "keep the run directory")

const (
	repoFromHere   = "../../../../.."
	configRelative = "demo/loop/demo.json"
	answersDir     = "testdata/answers"
	taskDir        = "testdata/task"
	fixDir         = "testdata/fix"
	exitAwaiting   = 3
	exitBlocked    = 1
	maxWaves       = 12
	answerFileMode = 0o644
	// weakRequirement is the requirement whose only test passes without constraining it.
	weakRequirement      = "BOX-2"
	survivingValidMutant = "surviving_valid_mutant"
)

// proofModules are the module binaries, in the order prove-all runs them, with the
// honest host of each.
func proofModules() []proofModule {
	hosts := []hostprototest.AnswerSource{boxoffice.Oracle{}, boxoffice.Alignment(), hostprototest.FirstOption{}}
	var found []proofModule
	for index, module := range modules.All() {
		found = append(found, proofModule{Module: module, answers: hosts[index]})
	}
	return found
}

type proofModule struct {
	modules.Module
	answers hostprototest.AnswerSource
}

func TestFixtureGate(t *testing.T) {
	t.Run("the gate blocks on the weak test of BOX-2 alone and passes once it reaches age 65 [verifies SP-2407~1]", gateBlocksThenPasses)
}

func gateBlocksThenPasses(t *testing.T) {
	repo, err := filepath.Abs(repoFromHere)
	if err != nil {
		t.Fatal(err)
	}
	layout := plan.Layout{Repo: repo, Run: runDir(t)}
	for _, binary := range []string{plan.ShallnotBinary, plan.VerifyBinary, "spec-proof-dp", "spec-proof-alignment", "spec-proof-completeness"} {
		if _, err := os.Stat(layout.Bin(binary)); err != nil {
			t.Skipf("%s is not built: run script/product-build", layout.Bin(binary))
		}
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 with pytest is required")
	}
	run := demoRun(t, repo)
	preparer := app.Preparer{Processes: adapters.Exec{}, Files: adapters.OSFiles{}, Env: os.Environ()}
	prepared, err := preparer.Prepare(context.Background(), layout, run, []string{layout.Bin(""), filepath.Dir(python)})
	if err != nil {
		t.Fatal(err)
	}

	hosts := newHosts(t)
	session := newSession(t, prepared, hosts)
	overlay(t, taskDir, layout.Project())
	before := session.proveAndGate(t)
	if before.exitCode != exitBlocked {
		t.Fatalf("with the weak test the gate exited %d, want %d:\n%s", before.exitCode, exitBlocked, before.output)
	}
	blocking := before.blockingFindings()
	if !slices.Contains(blocking, finding{survivingValidMutant, weakRequirement}) || slices.ContainsFunc(blocking, func(f finding) bool { return f.requirement != weakRequirement }) {
		t.Fatalf("with the weak test the blocking findings are %v, want a surviving valid mutant of %s and nothing on another requirement:\n%s", blocking, weakRequirement, before.output)
	}

	overlay(t, fixDir, layout.Project())
	after := session.proveAndGate(t)
	if after.exitCode != 0 {
		t.Fatalf("with the strengthened test the gate exited %d, want 0:\n%s", after.exitCode, after.output)
	}
	hosts.save(t)
}

// runDir is a fresh directory with its symbolic links resolved, as the module
// stores address a project by its resolved root.
func runDir(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	if *keep {
		var err error
		if parent, err = os.MkdirTemp("", "fixturegate-"); err != nil {
			t.Fatal(err)
		}
		t.Logf("run directory: %s", parent)
	}
	dir, err := filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func demoRun(t *testing.T, repo string) config.Run {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo, configRelative))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return config.Run{Task: parsed.Tasks[0], Arm: config.ArmFourProofs, Repetition: 1}
}

// overlay copies the files of a testdata tree over the project.
func overlay(t *testing.T, tree, project string) {
	t.Helper()
	err := filepath.WalkDir(tree, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		relative, err := filepath.Rel(tree, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(project, relative), content, answerFileMode)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// hosts answers the tickets of every module: from the recordings, or from the
// honest hosts while recording them under -update.
type hosts struct {
	recorded  map[hostproto.RequestID]string
	recording bool
	recorders []*hostprototest.Recorder
}

func newHosts(t *testing.T) *hosts {
	t.Helper()
	h := &hosts{recorded: map[hostproto.RequestID]string{}, recording: *update}
	if h.recording {
		return h
	}
	entries, err := os.ReadDir(answersDir)
	if err != nil {
		t.Fatalf("read the recorded answers (run the test with -update to write them): %v", err)
	}
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(answersDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		h.recorded[hostproto.RequestID(strings.TrimSuffix(entry.Name(), ".json"))] = string(content)
	}
	return h
}

func (h *hosts) source(host hostprototest.AnswerSource) hostprototest.AnswerSource {
	if !h.recording {
		return hostprototest.Recordings(h.recorded)
	}
	recorder := hostprototest.NewRecorder(host)
	h.recorders = append(h.recorders, recorder)
	return recorder
}

func (h *hosts) save(t *testing.T) {
	t.Helper()
	if !h.recording {
		return
	}
	if err := os.RemoveAll(answersDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(answersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, recorder := range h.recorders {
		for id, answer := range recorder.Answers {
			h.recorded[id] = answer
		}
	}
	for id, answer := range h.recorded {
		if err := os.WriteFile(filepath.Join(answersDir, string(id)+".json"), []byte(answer+"\n"), answerFileMode); err != nil {
			t.Fatal(err)
		}
	}
}

// moduleRunner runs a module binary in the project with the run's environment,
// as the simulated Claude Code runs it.
type moduleRunner struct {
	module modules.Module
	binary string
	dir    string
	env    []string
}

// Run implements claudecode.Module.
func (m moduleRunner) Run(stdin string, args ...string) claudecode.Result {
	command := exec.Command(m.binary, args...)
	command.Dir, command.Env, command.Stdin = m.dir, m.env, strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	result := claudecode.Result{Stdout: stdout.String(), Stderr: stderr.String()}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		result.Code = exit.ExitCode()
	case err != nil:
		result.Code, result.Stderr = -1, err.Error()
	}
	return result
}

// Server implements claudecode.Module.
func (m moduleRunner) Server() *exec.Cmd {
	command := exec.Command(m.binary, "mcp", "--role", "executor", "--harness", harness.FlagClaude)
	command.Dir, command.Env = m.dir, m.env
	return command
}

// ToolName implements claudecode.Module.
func (m moduleRunner) ToolName(tool string) string {
	return harness.Claude(m.module.Identity()).ToolName(tool)
}

func (m moduleRunner) prove(t *testing.T, args ...string) int {
	t.Helper()
	result := m.Run("", append([]string{"prove"}, args...)...)
	if result.Code != 0 && result.Code != exitAwaiting && result.Code != exitBlocked {
		t.Fatalf("%s prove %v exited %d:\n%s%s", m.module.Binary, args, result.Code, result.Stdout, result.Stderr)
	}
	return result.Code
}

type gateOutcome struct {
	exitCode int
	output   string
	report   gateReport
}

type gateReport struct {
	Findings []struct {
		Category      string `json:"category"`
		Blocking      bool   `json:"blocking"`
		RequirementID string `json:"requirement_id"`
	} `json:"findings"`
	Requirements []struct {
		ID       string `json:"id"`
		Revision int    `json:"revision"`
	} `json:"requirements"`
}

// finding is a blocking finding of the gate: its category and its requirement.
type finding struct{ category, requirement string }

// blockingFindings lists the blocking findings of the gate, sorted.
func (g gateOutcome) blockingFindings() []finding {
	var found []finding
	for _, reported := range g.report.Findings {
		if reported.Blocking {
			found = append(found, finding{reported.Category, reported.RequirementID})
		}
	}
	slices.SortFunc(found, func(a, b finding) int { return strings.Compare(a.category+a.requirement, b.category+b.requirement) })
	return found
}

// session is the simulated Claude Code session of the test: one host per module,
// started once, so that every executor it runs has an agent of its own.
type session struct {
	prepared app.Prepared
	hosts    *hosts
	modules  []servedModule
}

type servedModule struct {
	proofModule
	runner moduleRunner
	host   *claudecode.Host
}

func newSession(t *testing.T, prepared app.Prepared, hosts *hosts) *session {
	t.Helper()
	l := prepared.Layout
	s := &session{prepared: prepared, hosts: hosts}
	for _, module := range proofModules() {
		runner := moduleRunner{module: module.Module, binary: l.Bin(module.Binary), dir: l.Project(), env: prepared.Environment}
		s.modules = append(s.modules, servedModule{proofModule: module, runner: runner, host: claudecode.NewHost(t, runner, t.TempDir())})
	}
	return s
}

// proveAndGate runs prove-all as the skill does, with the hosts answering every
// wave, publishes the local proofs and runs the gate.
func (s *session) proveAndGate(t *testing.T) gateOutcome {
	t.Helper()
	l := s.prepared.Layout
	for _, module := range s.modules {
		code := module.runner.prove(t, "--changed")
		for wave := 0; code == exitAwaiting; wave++ {
			if wave == maxWaves {
				t.Fatalf("%s still awaits the host after %d waves", module.Binary, maxWaves)
			}
			module.host.Serve(s.hosts.source(module.answers))
			code = module.runner.prove(t, "--resume")
		}
	}
	verify := exec.Command(l.Bin(plan.VerifyBinary), "--accept-local", ".spec-proof/cache", "--out", ".spec-proof/verified")
	verify.Dir, verify.Env = l.Project(), s.prepared.Environment
	if output, err := verify.CombinedOutput(); err != nil {
		t.Fatalf("verify: %v\n%s", err, output)
	}
	reportPath := filepath.Join(l.Run, "gate.json")
	gate := exec.Command(l.Bin(plan.ShallnotBinary), plan.GateArgv(l, reportPath)[1:]...)
	gate.Dir, gate.Env = l.Project(), s.prepared.Environment
	output, err := gate.CombinedOutput()
	outcome := gateOutcome{output: string(output)}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		outcome.exitCode = exit.ExitCode()
	case err != nil:
		t.Fatalf("gate: %v", err)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("the gate wrote no report: %v\n%s", err, output)
	}
	if err := json.Unmarshal(data, &outcome.report); err != nil {
		t.Fatal(err)
	}
	return outcome
}
