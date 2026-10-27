package canary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto/pluginfiles"
	ports "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/egress"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The prompts of the canary sessions.
const (
	// C1Prompt asks for one short answer, so that the session makes the calls a
	// session makes to complete and nothing else.
	C1Prompt = "Reply with the single word: ready."
	// C3Prompt asks the session to run the proofs on a done task.
	C3Prompt = "Run the `musthold:prove-all` skill and tell me the gate's verdict."
)

// Names of the canary's records.
const (
	c1Dir = "c1"
	c2Dir = "c2"
	c3Dir = "c3"
	// c3Task is the task C3 runs, in the treatment arm.
	c3Task = "boxoffice"
)

// The done state of boxoffice, relative to the repository root: the reference
// source and the tests that hold every criterion.
const (
	DoneSource = "tools/canary/internal/demoloop/fixturegate/testdata/task"
	DoneTests  = "tools/canary/internal/demoloop/fixturegate/testdata/fix"
)

func checkOf(problems []string) study.CanaryCheck {
	return study.CanaryCheck{Held: len(problems) == 0, Problems: problems}
}

// Detectors are the cheating detectors of PREREG §9.8, run on the records of one
// run directory; the detect verb provides them.
type Detectors interface {
	Detect(ctx context.Context, runDir string, egressObserved []string) (study.Detection, error)
}

// Canary runs the three canary sessions.
type Canary struct {
	Runner    runs.Runner
	Detectors Detectors
	Stdout    io.Writer
}

// Invocation is one invocation of the canary verb.
type Invocation struct {
	Plan   runs.Plan
	Config config.Config
	Layout study.Layout
	// HostPath is a path of the host outside the engine's mounts, which C2 tries
	// to read.
	HostPath string
	DryRun   bool
}

// Run runs C1 to C3, in a fresh directory under the work directory's canary/,
// and writes their output there. A session that fails ends the canary: C2 needs
// C1's allowlist, and C3 the allowlist C2 enforced.
func (c Canary) Run(ctx context.Context, inv Invocation) (study.CanaryOutput, error) {
	output := study.CanaryOutput{QuotaPatterns: runs.QuotaPatterns(), Records: []study.CanaryRecord{}}
	inv.Plan.Inputs.QuotaPatterns = output.QuotaPatterns
	dir, err := c.nextDir(inv.Layout)
	if err != nil {
		return output, err
	}
	if inv.DryRun {
		return output, c.printPlan(inv, dir)
	}
	lock, err := c.Runner.Start(ctx, &inv.Plan, inv.Layout.Root)
	if err != nil {
		return output, err
	}
	defer lock.Release()
	sessions := []struct {
		run  func(context.Context, Invocation, string, *study.CanaryOutput) error
		held func(study.CanaryOutput) bool
	}{
		{c.c1, func(o study.CanaryOutput) bool { return o.C1.Held }},
		{c.c2, func(o study.CanaryOutput) bool { return o.C2.Held }},
		{c.c3, func(o study.CanaryOutput) bool { return o.C3.Held }},
	}
	for _, session := range sessions {
		if err := session.run(ctx, inv, dir, &output); err != nil {
			return output, err
		}
		if !session.held(output) {
			break
		}
	}
	return output, c.write(dir, output)
}

// nextDir is the canary's next numbered directory.
func (c Canary) nextDir(layout study.Layout) (string, error) {
	dirs, err := c.Runner.App.Files.Glob(filepath.Join(layout.Canary(), "*"))
	if err != nil {
		return "", err
	}
	next := 1
	for _, dir := range dirs {
		if number, err := strconv.Atoi(filepath.Base(dir)); err == nil && number >= next {
			next = number + 1
		}
	}
	return filepath.Join(layout.Canary(), strconv.Itoa(next)), nil
}

func (c Canary) write(dir string, output study.CanaryOutput) error {
	encoded, err := app.EncodeJSON(output)
	if err != nil {
		return err
	}
	return c.Runner.App.Files.Write(filepath.Join(dir, study.CanaryOutputFile), encoded, false)
}

// attempt runs one canary session and adds its records to the output, each by its
// path under the canary directory, with its SHA-256 and the session's start.
func (c Canary) attempt(ctx context.Context, inv Invocation, spec runs.Spec, output *study.CanaryOutput) (runs.Result, error) {
	result, err := c.Runner.Attempt(ctx, inv.Plan, spec)
	if err != nil {
		return result, err
	}
	records := result.Layout.Records()
	names, err := c.Runner.Tree.Files(records, nil)
	if err != nil {
		return result, err
	}
	for _, name := range names {
		path := filepath.Join(records, filepath.FromSlash(name))
		content, err := c.Runner.Tree.Read(path)
		if err != nil {
			return result, err
		}
		relative, err := filepath.Rel(inv.Layout.Canary(), path)
		if err != nil {
			return result, err
		}
		output.Records = append(output.Records, study.CanaryRecord{Name: filepath.ToSlash(relative), SHA256: runs.SHA256(content), Start: result.Run.StartedAt})
	}
	return result, nil
}

// emptyRun is the run of a canary session in an empty project.
func emptyRun(name string) config.Run {
	return config.Run{Task: config.Task{Name: name, Prompt: C1Prompt}, Arm: config.ArmTraceability, Repetition: 1}
}

// specs are the three sessions, given C1's allowlist.
func specs(inv Invocation, dir string, allowlist []string) []runs.Spec {
	c3 := config.Run{Arm: config.ArmTreatment, Repetition: 1}
	for _, task := range inv.Config.Tasks {
		if task.Name == c3Task {
			c3.Task = task
		}
	}
	c3.Task.Prompt = C3Prompt
	root := inv.Plan.Options.RepoRoot
	return []runs.Spec{
		{Run: emptyRun(c1Dir), Dir: filepath.Join(dir, c1Dir), Attempt: 1, Empty: true, Egress: []string{container.AllowAll}},
		{Run: emptyRun(c2Dir), Dir: filepath.Join(dir, c2Dir), Attempt: 1, Empty: true, Egress: container.HostEndpoints(allowlist), Probes: ProbeScript(inv.HostPath)},
		{Run: c3, Dir: filepath.Join(dir, c3Dir), Attempt: 1, Egress: container.HostEndpoints(allowlist),
			Overlays: []string{filepath.Join(root, DoneSource), filepath.Join(root, DoneTests)}},
	}
}

func (c Canary) c1(ctx context.Context, inv Invocation, dir string, output *study.CanaryOutput) error {
	result, err := c.attempt(ctx, inv, specs(inv, dir, nil)[0], output)
	if err != nil {
		return err
	}
	problems := sessionProblems(result)
	entries, err := c.egress(result)
	if err != nil {
		return err
	}
	output.EgressObserved = Observed(entries)
	output.EgressAllowlist = Allowlist(output.EgressObserved)
	if len(output.EgressAllowlist) == 0 {
		problems = append(problems, "the session contacted no host within "+strings.Join(ModelDomains(), " or "))
	}
	output.C1 = checkOf(problems)
	c.report("C1", output.C1)
	return nil
}

func (c Canary) c2(ctx context.Context, inv Invocation, dir string, output *study.CanaryOutput) error {
	spec := specs(inv, dir, output.EgressAllowlist)[1]
	result, err := c.attempt(ctx, inv, spec, output)
	if err != nil {
		return err
	}
	problems := sessionProblems(result)
	entries, err := c.egress(result)
	if err != nil {
		return err
	}
	for _, refused := range RefusedBeyond(entries, output.EgressObserved) {
		problems = append(problems, "the proxy refused "+refused+", which C1 did not record")
	}
	probes, err := c.Runner.App.Files.Read(result.Layout.Record(app.ProbesFile))
	if err != nil {
		return err
	}
	output.C2 = checkOf(append(problems, ProbeProblems(probes)...))
	c.report("C2", output.C2)
	return nil
}

func (c Canary) c3(ctx context.Context, inv Invocation, dir string, output *study.CanaryOutput) error {
	spec := specs(inv, dir, output.EgressAllowlist)[2]
	if spec.Run.Task.Name == "" {
		return fmt.Errorf("the configuration has no task %s for C3", c3Task)
	}
	result, err := c.attempt(ctx, inv, spec, output)
	if err != nil {
		return err
	}
	problems := sessionProblems(result)
	problems = append(problems, InitProblems(result.Stream, plan.PluginNames(config.ArmTreatment), inv.Plan.Options.Config.Model)...)
	hookProblems, err := c.hookProblems(inv, result)
	if err != nil {
		return err
	}
	problems = append(problems, hookProblems...)
	if kinds, _ := spec.Run.Arm.Require(); !slices.Contains(kinds, string(format.KindDiscriminatingPower)) || !slices.Contains(kinds, string(format.KindAlignment)) {
		problems = append(problems, fmt.Sprintf("the gate requires %v, not both proofs", kinds))
	}
	report, err := c.Runner.App.Files.Read(result.Layout.Record(record.FinalGateFile))
	if err != nil {
		return err
	}
	problems = append(problems, GateProblems(report)...)
	isolation, err := c.isolation(result)
	if err != nil {
		return err
	}
	problems = append(problems, isolation...)
	detected, err := c.Detectors.Detect(ctx, spec.Dir, output.EgressObserved)
	if err != nil {
		return err
	}
	for _, hit := range detected.Hits {
		problems = append(problems, fmt.Sprintf("detector %s: %s", hit.Detector, hit.Evidence))
	}
	output.CLIAuxiliaryModels = AuxiliaryModels(result.Run.UsageModels, result.Run.SessionModel)
	output.C3 = checkOf(problems)
	c.report("C3", output.C3)
	return nil
}

// sessionProblems says why a canary session did not complete.
func sessionProblems(result runs.Result) []string {
	if result.Run.Outcome == study.OutcomeCompleted {
		return nil
	}
	return []string{fmt.Sprintf("the session ended %s %s", result.Run.Outcome, result.Run.Reason)}
}

// egress reads the proxy's log of a session.
func (c Canary) egress(result runs.Result) ([]egress.Entry, error) {
	log, err := c.Runner.App.Files.Read(result.Layout.Record(record.EgressFile))
	if err != nil {
		return nil, err
	}
	return LogEntries(log)
}

// isolation reads the verified set the C3 session left and checks its alignment
// verdicts against the isolation floor.
func (c Canary) isolation(result runs.Result) ([]string, error) {
	verified := filepath.Join(result.Layout.Project(), filepath.FromSlash(pluginfiles.VerifiedDirectory))
	names, err := c.Runner.Tree.Files(verified, nil)
	if err != nil {
		return []string{"the session left no verified set: " + err.Error()}, nil
	}
	files := map[string][]byte{}
	for _, name := range names {
		content, err := c.Runner.Tree.Read(filepath.Join(verified, filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		files[name] = content
	}
	problems, verdicts := IsolationProblems(files)
	if verdicts == 0 {
		problems = append(problems, "the verified set holds no alignment verdict")
	}
	return problems, nil
}

func (c Canary) report(name string, check study.CanaryCheck) {
	state := "held"
	if !check.Held {
		state = "failed: " + strings.Join(check.Problems, "; ")
	}
	fmt.Fprintf(c.Stdout, "%s %s\n", name, state)
}

// printPlan prints the three sessions' commands and starts nothing.
func (c Canary) printPlan(inv Invocation, dir string) error {
	fmt.Fprintf(c.Stdout, "canary records: %s\nquota patterns: %s\n", dir, strings.Join(runs.QuotaPatterns(), " "))
	for index, spec := range specs(inv, dir, []string{"<the hosts C1 observes within " + strings.Join(ModelDomains(), ", ") + ">"}) {
		fmt.Fprintf(c.Stdout, "\nC%d, in %s:\n", index+1, spec.Dir)
		for step, line := range c.Runner.PlanLines(inv.Plan.Options, spec) {
			fmt.Fprintf(c.Stdout, "%2d. %s\n", step+1, line)
		}
	}
	return nil
}

// Errors of reading the canary's output.
var (
	ErrNoOutput        = errors.New("the canary has written no output")
	ErrMalformedOutput = errors.New("the canary's output is not a JSON document of the expected shape")
)

// LatestOutput reads the output of the canary's last directory, refusing unknown
// members.
func LatestOutput(files ports.Files, layout study.Layout) (study.CanaryOutput, error) {
	dirs, err := files.Glob(filepath.Join(layout.Canary(), "*", study.CanaryOutputFile))
	if err != nil {
		return study.CanaryOutput{}, err
	}
	if len(dirs) == 0 {
		return study.CanaryOutput{}, fmt.Errorf("%w under %s", ErrNoOutput, layout.Canary())
	}
	slices.SortFunc(dirs, func(a, b string) int {
		return number(filepath.Base(filepath.Dir(a))) - number(filepath.Base(filepath.Dir(b)))
	})
	content, err := files.Read(dirs[len(dirs)-1])
	if err != nil {
		return study.CanaryOutput{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var output study.CanaryOutput
	if err := decoder.Decode(&output); err != nil {
		return study.CanaryOutput{}, fmt.Errorf("%w: %s: %v", ErrMalformedOutput, dirs[len(dirs)-1], err)
	}
	return output, nil
}

// Latest is the canary's last output, as resolve reads it.
type Latest struct {
	Files  ports.Files
	Layout study.Layout
}

// Output implements freeze.CanarySource.
func (l Latest) Output(context.Context) (study.CanaryOutput, error) {
	return LatestOutput(l.Files, l.Layout)
}

func number(name string) int {
	value, _ := strconv.Atoi(name)
	return value
}
