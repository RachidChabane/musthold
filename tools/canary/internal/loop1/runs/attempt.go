package runs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/startup"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// ErrAttemptExists refuses to reuse an attempt's directory: every attempt starts
// in a fresh one (PREREG §6.4).
var ErrAttemptExists = errors.New("the attempt's directory already exists")

// Runner runs attempts through the stages of the agent-loop runner.
type Runner struct {
	App        app.Runner
	Tree       Tree
	Deviations DeviationWriter
}

// Plan is what every attempt of one invocation shares.
type Plan struct {
	// Options are the agent-loop runner's options, with the study's profile.
	Options app.Options
	Inputs  study.Inputs
	// ImageID is the id of the study's image as the engine names it.
	ImageID string
	Source  canary.ClaudeCredential
}

// Spec is one attempt.
type Spec struct {
	Run config.Run
	// Dir is the attempt's run directory, which must not exist yet; Attempt is
	// its number.
	Dir     string
	Attempt int
	// Empty runs the session in an empty project, with no preparation and no
	// collection of a change.
	Empty bool
	// Overlays are directories copied over the equipped project, uncommitted,
	// before the session.
	Overlays []string
	// Expect are the task's frozen values, compared with the attempt's; nil
	// compares nothing.
	Expect *study.TaskInputs
	// Egress are the endpoints the proxy forwards to, and Probes what the
	// session's container runs after the CLI.
	Egress []string
	Probes string
}

// Result is how an attempt ended.
type Result struct {
	Run    study.Run
	Layout plan.Layout
	// Ran is set when the session ran, so that its stream is in the records.
	Ran bool
	// Stream is the decoded stream of the session.
	Stream evidence.ClaudeStream
	// Provenance lists what differs from the inputs freeze without voiding the
	// attempt.
	Provenance []string
	// TokenMatches is how many matches of the token the scan replaced.
	TokenMatches int
}

// Collection and record names of an attempt.
const (
	budgetSubtype = "error_max_budget_usd"
	// ShallnotVersionKey is the versions entry of `shallnot --version`.
	ShallnotVersionKey = "shallnot --version"
	sha256Program      = "sha256sum"
	versionFlag        = "--version"
	gitDirName         = ".git"
	attemptTimeout     = 5 * time.Minute
)

// attempt is one attempt in progress.
type attempt struct {
	r        Runner
	plan     Plan
	spec     Spec
	options  app.Options
	current  *app.Attempt
	preparer app.Preparer
	prepared app.Prepared
	session  *app.Session
	meta     record.Meta
	facts    Facts
	result   Result
	quota    *QuotaWatch
	state    *container.State
}

// Attempt runs one attempt: it prepares the project, conducts the session,
// collects what it left, judges its outcome and writes its records. A failure of
// the harness is the attempt's outcome; an error is returned only when the
// attempt's records cannot be written.
func (r Runner) Attempt(ctx context.Context, p Plan, s Spec) (Result, error) {
	if r.App.Files.Exists(s.Dir) {
		return Result{}, fmt.Errorf("%w: %s", ErrAttemptExists, s.Dir)
	}
	options := p.Options
	options.Profile.Egress, options.Profile.Probes = s.Egress, s.Probes
	current, err := r.App.Open(options, s.Dir)
	if err != nil {
		return Result{}, err
	}
	defer current.Close(ctx)
	quota, err := NewQuotaWatch(p.Inputs.QuotaPatterns, r.App.Clock.Now)
	if err != nil {
		return Result{}, err
	}
	a := &attempt{r: r, plan: p, spec: s, options: options, current: current, preparer: r.App.Preparer(current), quota: quota}
	a.result.Layout = current.Layout
	a.facts.Harness = a.run(ctx)
	return a.finish(ctx)
}

// run prepares, readies, conducts and collects, and returns why the harness
// failed the attempt, or an empty string.
func (a *attempt) run(ctx context.Context) string {
	for _, stage := range []func(context.Context) string{a.prepare, a.ready, a.conduct, a.collect} {
		if why := stage(ctx); why != "" {
			return why
		}
	}
	return ""
}

func (a *attempt) prepare(ctx context.Context) string {
	l := a.current.Layout
	for _, dir := range append(l.Dirs(), l.Project()) {
		if err := a.r.App.Files.MkdirAll(dir); err != nil {
			return "create the run directory: " + err.Error()
		}
	}
	versions, err := a.r.App.Versions(ctx, a.current, a.options)
	a.meta.Versions = versions
	if err != nil {
		return err.Error()
	}
	versions[ShallnotVersionKey] = a.ask(ctx, l.Bin(plan.ShallnotBinary), versionFlag)
	a.meta.ClaudeSHA256, _, _ = strings.Cut(a.ask(ctx, sha256Program, l.Claude()), " ")
	a.toolchains(ctx, versions)
	if a.spec.Empty {
		return ""
	}
	prepared, err := a.preparer.Prepare(ctx, l, a.spec.Run, app.LeadingDirs(l))
	a.prepared = prepared
	if err != nil {
		return err.Error()
	}
	return a.equipped(ctx)
}

// toolchains records, under the keys of the inputs freeze, the version of each
// toolchain of the image and of the engine (PREREG §2).
func (a *attempt) toolchains(ctx context.Context, versions map[string]string) {
	for key, program := range ImageToolchains() {
		versions[key] = DottedVersion(a.ask(ctx, program, versionFlag))
	}
	engine, err := a.r.App.EngineVersion(ctx, a.options)
	if err != nil {
		engine = "unavailable: " + err.Error()
	}
	versions[freeze.ToolchainDocker] = DottedVersion(engine)
}

// ImageToolchains are the programs of the image whose version each run records,
// by the key of the inputs freeze.
func ImageToolchains() map[string]string {
	return map[string]string{
		freeze.ToolchainPython: "python3", freeze.ToolchainPytest: "pytest", freeze.ToolchainUV: "uv",
		freeze.ToolchainNode: "node", freeze.ToolchainGit: plan.GitBinary,
	}
}

// dottedVersion is the first dotted number of a version line.
var dottedVersion = regexp.MustCompile(`[0-9]+(\.[0-9]+)+`)

// DottedVersion is the first dotted number of a version line, as the inputs
// freeze records a toolchain's version, or the line itself when it holds none.
func DottedVersion(line string) string {
	if version := dottedVersion.FindString(line); version != "" {
		return version
	}
	return line
}

// ask runs a command in a step container and returns what it printed.
func (a *attempt) ask(ctx context.Context, argv ...string) string {
	output, err := a.current.Steps().Run(ctx, canary.CommandSpec{Argv: argv, Dir: a.current.Layout.Run, Env: a.current.Environment(), Timeout: attemptTimeout})
	if err != nil {
		return "unavailable: " + err.Error()
	}
	return strings.TrimSpace(output.Stdout)
}

// equipped records the base commit, the equipped commit and the equipped tree's
// digest, and fails an attempt whose values differ from the inputs freeze.
func (a *attempt) equipped(ctx context.Context) string {
	l := a.current.Layout
	facts, err := equippedFacts(ctx, a.preparer, &a.prepared, a.r.Tree)
	if err != nil {
		return err.Error()
	}
	run := &a.result.Run
	run.BaseCommit, run.EquippedCommit, run.EquippedTree = facts.BaseCommit, facts.Commit, facts.Tree
	if why := a.spec.differs(facts); why != "" {
		return why
	}
	for _, overlay := range a.spec.Overlays {
		if err := a.r.App.Files.CopyTree(overlay, l.Project()); err != nil {
			return "overlay " + overlay + ": " + err.Error()
		}
	}
	return ""
}

// differs says how an attempt's equipped project differs from the inputs freeze.
func (s Spec) differs(facts Equipped) string {
	if s.Expect == nil {
		return ""
	}
	if facts.BaseCommit != s.Expect.BaseCommit {
		return fmt.Sprintf("the base commit is %s, the inputs freeze holds %s", facts.BaseCommit, s.Expect.BaseCommit)
	}
	if want := s.Expect.EquippedTree[string(s.Run.Arm)]; facts.Tree != want {
		return fmt.Sprintf("the equipped tree's digest is %s, the inputs freeze holds %s", facts.Tree, want)
	}
	return ""
}

func (a *attempt) ready(ctx context.Context) string {
	if a.spec.Empty {
		a.prepared = app.Prepared{Layout: a.current.Layout, Environment: a.current.Environment()}
	}
	session, err := a.r.App.Ready(ctx, a.options, a.current, a.preparer, &a.prepared, a.spec.Run)
	a.session = session
	if err != nil {
		return err.Error()
	}
	if err := a.hashes(study.HashesBeforeFile); err != nil {
		return err.Error()
	}
	a.meta = a.describe()
	return ""
}

// describe is the run's description before its session starts.
func (a *attempt) describe() record.Meta {
	meta := app.MetaOf(a.options, a.current, a.spec.Run, a.session, a.meta.Versions)
	meta.ClaudeSHA256 = a.meta.ClaudeSHA256
	return meta
}

func (a *attempt) conduct(ctx context.Context) string {
	check := startup.New(plan.PluginNames(a.spec.Run.Arm))
	stops := func(line []byte) string {
		if why := check.Line(line); why != "" {
			return why
		}
		return a.quota.Line(line)
	}
	conducted, err := a.r.App.Conduct(ctx, a.options, a.current, a.session, a.plan.Source, stops, &a.meta)
	a.meta.EndedAt, a.meta.ExitCode, a.meta.Killed = a.r.App.Clock.Now(), conducted.Outcome.ExitCode, conducted.Outcome.Killed
	a.result.Ran = !a.meta.StartedAt.IsZero()
	a.state = conducted.State
	if startup.Failed(conducted.Outcome.Killed) {
		a.facts.Killed = conducted.Outcome.Killed
		return "the session did not start: " + conducted.Outcome.Killed + ": " + check.Problem()
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// collect records the project the session left: its hashes, its change and the
// acceptance gate's report.
func (a *attempt) collect(ctx context.Context) string {
	if err := a.hashes(study.HashesAfterFile); err != nil {
		return err.Error()
	}
	if a.spec.Empty {
		return ""
	}
	collector := app.Collector{Processes: a.current.Steps(), Files: a.r.App.Files, Preparer: a.preparer}
	if err := collector.Change(ctx, &a.prepared); err != nil {
		return err.Error()
	}
	return a.accept(ctx, collector)
}

// accept runs the acceptance gate, and puts the session's files back whatever the
// restore and the gate did; a restore or a put-back that fails, or a gate that
// cannot run, is a harness error.
func (a *attempt) accept(ctx context.Context, collector app.Collector) string {
	l := a.current.Layout
	acceptance := plan.AcceptanceSteps(l, a.result.Run.EquippedCommit, l.Record(record.FinalGateFile))
	why := a.step(ctx, collector, acceptance.Restore, true)
	if why == "" {
		why = a.step(ctx, collector, acceptance.Gate, false)
	}
	if putBack := a.step(ctx, collector, acceptance.PutBack, true); why == "" {
		why = putBack
	}
	return why
}

// step runs one collection step and says why it failed: it could not run, or,
// when it must succeed, it exited non-zero.
func (a *attempt) step(ctx context.Context, collector app.Collector, step plan.Step, mustSucceed bool) string {
	outcomes, err := collector.Steps(ctx, &a.prepared, []plan.Step{step})
	if err != nil {
		return err.Error()
	}
	if outcome := outcomes[0]; mustSucceed && outcome.ExitCode != 0 {
		return fmt.Sprintf("%s exited %d: %s", outcome.Purpose, outcome.ExitCode, strings.TrimSpace(outcome.Stderr))
	}
	return ""
}

// hashes writes the SHA-256 of every file of the project, git-ignored files
// included and the git directory left out.
func (a *attempt) hashes(name string) error {
	project := a.current.Layout.Project()
	files, err := a.r.Tree.Files(project, []string{gitDirName})
	if err != nil {
		return err
	}
	hashes := map[string]string{}
	for _, file := range files {
		content, err := a.r.Tree.Read(filepath.Join(project, filepath.FromSlash(file)))
		if err != nil {
			return err
		}
		hashes[file] = SHA256(content)
	}
	return a.writeRecord(name, hashes)
}

func (a *attempt) writeRecord(name string, value any) error {
	encoded, err := app.EncodeJSON(value)
	if err != nil {
		return err
	}
	return a.r.App.Files.Write(a.current.Layout.Record(name), encoded, false)
}

// finish reads the stream, judges the outcome, and writes run.json, container.json,
// the token scan and the transcript.
func (a *attempt) finish(ctx context.Context) (Result, error) {
	l := a.current.Layout
	if err := app.WriteSteps(a.r.App.Files, l.Record(record.StepsFile), a.prepared.Steps); err != nil {
		return a.result, err
	}
	if a.result.Ran {
		if err := a.readStream(); err != nil {
			return a.result, err
		}
	}
	if a.state != nil {
		a.facts.OOMKilled = a.state.OOMKilled
		if err := a.writeRecord(study.ContainerFile, study.ContainerState{ImageDigest: a.plan.ImageID, ExitCode: a.state.ExitCode, OOMKilled: a.state.OOMKilled}); err != nil {
			return a.result, err
		}
	}
	run := &a.result.Run
	run.Meta, run.Attempt = a.meta, a.spec.Attempt
	if run.Meta.Task == "" {
		run.Meta.Task, run.Meta.Arm, run.Meta.Repetition = a.spec.Run.Task.Name, string(a.spec.Run.Arm), a.spec.Run.Repetition
		run.Meta.Model, run.Meta.Prompt = a.options.Config.Model, a.spec.Run.Task.Prompt
	}
	run.Outcome, run.Reason = Classify(a.facts)
	if stopped, reset := a.quota.Stopped(); stopped && run.Outcome == study.OutcomeQuotaStop {
		run.QuotaReset = reset
	}
	a.result.Provenance = Provenance(a.plan.Inputs, run.Meta.Versions, run.Meta.ClaudeSHA256, run.SessionModel, run.UsageModels)
	if err := a.writeRecord(record.MetaFile, run); err != nil {
		return a.result, err
	}
	matches, err := a.scan()
	if err != nil {
		return a.result, err
	}
	a.result.TokenMatches = matches
	if a.result.Ran {
		if err := a.r.App.Render(l); err != nil {
			return a.result, err
		}
	}
	return a.result, nil
}

// readStream decodes the session's stream into the facts the outcome is judged on.
func (a *attempt) readStream() error {
	content, err := a.r.App.Files.Read(a.current.Layout.Record(record.StreamFile))
	if err != nil {
		return err
	}
	lines, err := evidence.Lines(record.StreamFile, content)
	if err != nil {
		return err
	}
	stream := evidence.DecodeClaude(lines)
	a.result.Stream = stream
	for _, line := range lines {
		a.quota.Line(line.Raw)
	}
	a.facts.Quota, _ = a.quota.Stopped()
	if a.facts.Killed == "" {
		a.facts.Killed = a.meta.Killed
	}
	run := &a.result.Run
	if init, found := stream.Init(); found {
		a.facts.Started, run.SessionModel = true, init.Init.Model
		if run.SessionModel != a.options.Config.Model {
			a.facts.ModelMismatch = run.SessionModel
		}
	}
	if final, found := stream.Final(); found {
		run.UsageModels = slices.Sorted(slices.Values(final.Result.Models))
		a.facts.BudgetCapped = final.Result.Subtype == budgetSubtype
	}
	return nil
}

// scan replaces the token in every record and writes the token scan.
func (a *attempt) scan() (int, error) {
	records := a.current.Layout.Records()
	files, err := a.r.Tree.Files(records, nil)
	if err != nil {
		return 0, err
	}
	scan := study.TokenScan{Scanned: files, Replaced: map[string]int{}}
	total := 0
	for _, file := range files {
		path := filepath.Join(records, filepath.FromSlash(file))
		content, err := a.r.Tree.Read(path)
		if err != nil {
			return 0, err
		}
		redacted, count := record.Redact(content, a.token())
		if count == 0 {
			continue
		}
		scan.Replaced[file], total = count, total+count
		if err := a.r.App.Files.Write(path, redacted, false); err != nil {
			return 0, err
		}
	}
	return total, a.writeRecord(study.TokenScanFile, scan)
}

// PlanLines are the commands of one attempt, in order, as a dry run prints them.
func (r Runner) PlanLines(o app.Options, s Spec) []string {
	o.Profile.Egress, o.Profile.Probes = s.Egress, s.Probes
	planned := r.App.SessionPlan(o, s.Run, s.Dir, !s.Empty)
	l := planned.Layout
	lines := append([]string{
		filepath.Join(o.RepoRoot, plan.MemGuardRelative) + " check   (on the host)",
		planned.Step([]string{l.Bin(plan.ShallnotBinary), versionFlag}),
		planned.Step([]string{sha256Program, l.Claude()}) + "   (recorded in run.json)",
	}, planned.Lines...)
	if !s.Empty {
		for _, argv := range [][]string{plan.BaseCommitArgv(), plan.HeadCommitArgv(), plan.TrackedFilesArgv()} {
			lines = append(lines, planned.Step(argv)+"   (compared with inputs.json)")
		}
	}
	for _, overlay := range s.Overlays {
		lines = append(lines, "copy "+overlay+" over "+l.Project()+"   (on the host, uncommitted)")
	}
	lines = append(lines, "write "+l.Record(study.HashesBeforeFile)+" and, once the session has ended, "+l.Record(study.HashesAfterFile))
	if !s.Empty {
		for _, argv := range plan.ChangeArgv() {
			lines = append(lines, planned.Step(argv))
		}
		for _, step := range plan.AcceptanceSteps(l, "<equipped commit>", l.Record(record.FinalGateFile)).Steps() {
			lines = append(lines, planned.Step(step.Argv)+"   ("+step.Purpose+")")
		}
	}
	return append(lines,
		"write "+l.Record(study.ContainerFile)+", "+l.Record(record.MetaFile)+" and "+l.Record(study.TokenScanFile)+", every match of the token replaced by "+record.TokenPlaceholder,
		"write "+l.Record(record.TranscriptMarkdown)+" and "+l.Record(record.TranscriptHTML),
	)
}

// token is the token the session received, or empty when no session ran.
func (a *attempt) token() string {
	if a.session == nil {
		return ""
	}
	return a.session.Token()
}
