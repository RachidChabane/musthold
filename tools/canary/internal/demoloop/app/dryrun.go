package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/launch"
)

// printPlan prints what a live run would do: the window, where the token would
// come from, and for each run the exact commands in order, every one that touches
// the project a docker command, with the run directory left as a placeholder and
// the token shown as reaching the session's standard input. It reads no
// credential value and starts nothing.
func (r Runner) printPlan(o Options, now time.Time, source canary.ClaudeCredential, credentialErr error) error {
	length := time.Duration(o.Config.Caps.Seconds) * time.Second
	state := "a session started now fits"
	if !r.Window.Holds(now, length) {
		state = "a session started now does not fit (--force-window needed)"
	}
	fmt.Fprintf(r.Stdout, "now: %s, window %s, session cap %s: %s\n", now.Format("15:04"), r.Window.Describe(), length, state)
	switch {
	case credentialErr != nil:
		fmt.Fprintf(r.Stdout, "credentials: %v\n", credentialErr)
	case source == nil:
		fmt.Fprintf(r.Stdout, "credentials: none (store a token in the keychain as service %s, or set %s)\n", canary.KeychainService, launch.OAuthTokenVariable)
	default:
		fmt.Fprintf(r.Stdout, "credentials: source %s, read once per session and written to the entry's standard input (%s)\n", source.Source(), container.TokenPlaceholder)
	}
	fmt.Fprintf(r.Stdout, "lock: %s\nwork: %s\nproduct (read-only): %s\nimage: %s\negress allowlist: %s\n",
		filepath.Join(o.RepoRoot, canary.LockRelative), o.Work, o.Product, o.Image, strings.Join(r.layout(o, o.Work).Box.Endpoints(), ", "))
	runs := o.Config.Runs()
	fmt.Fprintf(r.Stdout, "runs, in order: %d\n", len(runs))
	for index, configured := range runs {
		fmt.Fprintf(r.Stdout, "\nrun %d: %s (task %s, arm %s, repetition %d)\n", index+1, configured.Name(), configured.Task.Name, configured.Arm, configured.Repetition)
		for step, line := range r.planLines(o, configured) {
			fmt.Fprintf(r.Stdout, "%2d. %s\n", step+1, line)
		}
	}
	fmt.Fprintln(r.Stdout, "\noutside the work area a run writes: nothing but the lock; every command that touches the project runs in a container")
	return nil
}

// planLines are the commands of one run, in order.
func (r Runner) planLines(o Options, configured config.Run) []string {
	dir := filepath.Join(o.Work, runDirPrefix+configured.Name()+"-"+placeholderRun)
	planned := r.SessionPlan(o, configured, dir)
	l := planned.Layout
	lines := append([]string{
		quoted(filepath.Join(o.RepoRoot, plan.MemGuardRelative), "check") + "   (on the host)",
		"sha256 " + filepath.Join(o.Product, plan.ProductClaudeDir, plan.ClaudeBinary) + "   (recorded in run.json)",
	}, planned.Lines...)
	for _, argv := range plan.ChangeArgv() {
		lines = append(lines, planned.Step(argv))
	}
	return append(lines,
		"write "+l.Record(record.ChangeFile)+"   (what the git diff printed)",
		planned.Step(plan.GateArgv(l, l.Record(record.FinalGateFile))),
		"scan every record for the token and for any text of a token's shape; a hit fails the run",
		"write "+l.Record(record.TranscriptMarkdown)+" and "+l.Record(record.TranscriptHTML),
	)
}

// Planned is what a run starts up to the removal of its session's containers,
// as the lines a dry run prints, with what renders any further step the same way.
type Planned struct {
	Layout plan.Layout
	Lines  []string
	// Step renders a command run in the project, in a step container of its own.
	Step func(argv []string) string
}

// SessionPlan lists, in order, the commands of one run in the run directory
// given, from the version check of the CLI to the removal of the session's
// containers and network.
func (r Runner) SessionPlan(o Options, configured config.Run, dir string) Planned {
	current := r.layout(o, dir)
	l := current.Layout
	docker := strings.Join(o.Docker, " ")
	step := func(argv []string) string {
		current.steps.count++
		env := plan.Environment(ContainerBase(l), plan.Variables(l, LeadingDirs(l)))
		if argv[0] == plan.GitBinary {
			env = append(env, plan.GitEnvironment()...)
		}
		return container.Line(docker, current.Box.StepArgv(current.steps.count, l.Project(), env, argv), false)
	}
	lines := []string{
		step([]string{l.Claude(), versionArgument}) + "   (must begin with " + o.ClaudePin + ")",
		fmt.Sprintf("copy %s -> %s   (on the host)", filepath.Join(o.RepoRoot, configured.Task.Fixture), l.Project()),
	}
	for _, planned := range append(plan.BaseSteps(), plan.EquipSteps(l, configured.Arm)...) {
		lines = append(lines, step(planned.Argv))
	}
	if configured.Arm.WithModules() && len(configured.Task.Operations) > 0 {
		block, _ := plan.BindOperations(nil, configured.Task.Operations)
		lines = append(lines, "append to "+filepath.Join(l.Project(), plan.MustholdConfig)+":\n      "+strings.ReplaceAll(strings.TrimSpace(string(block)), "\n", "\n      "))
	}
	if kinds, explicit := configured.Arm.Require(); explicit {
		lines = append(lines, "set proofs.require of "+filepath.Join(l.Project(), plan.ShallnotConfig)+" to ["+strings.Join(kinds, ", ")+"]")
	}
	for _, planned := range plan.CommitEquipSteps() {
		lines = append(lines, step(planned.Argv))
	}
	settings := "permissions.allow: " + strings.Join(o.Config.AllowedTools, ", ")
	if len(o.Profile.Deny) > 0 {
		settings += "; permissions.deny: " + strings.Join(o.Profile.Deny, ", ")
	}
	lines = append(lines,
		step(plan.GateArgv(l, l.Record(record.InitialGateFile))),
		"write "+l.Path(plan.ClaudeConfigDir, launch.ClaudeGlobalConfigFile)+"   (the project trusted)",
		"write "+l.Path(plan.SessionSettings)+"   ("+settings+")",
	)
	for _, name := range plan.Shimmed(configured.Arm) {
		line := "write " + l.Path(plan.ShimDir, name) + "   (records the hook calls of " + l.Bin(name) + " in " + l.Record(record.HooksFile)
		if o.Profile.CaptureStops && name == plan.ShallnotBinary {
			line += "; after each end-of-turn call, `" + plan.ShallnotBinary + " check --json-out` into " + filepath.Join(l.Stops(), "<n>", plan.StopReportFile) +
				" and the project's tree id into " + filepath.Join(l.Stops(), "<n>", plan.StopTreeFile)
		}
		lines = append(lines, line+")")
	}
	session := SessionOf(o, current, configured)
	lines = append(lines,
		container.Line(docker, current.Box.NetworkArgv(), false)+"   (no route out)",
		container.Line(docker, current.Box.ProxyArgv(l.Record(record.EgressFile)), false),
		container.Line(docker, current.Box.ConnectArgv(), false),
		container.Line(session.argv[0], session.argv[1:], true),
		fmt.Sprintf("caps: %d assistant messages (the subagents' included), %d s, %s USD; the docker client's process group ends at a cap",
			o.Config.Caps.AssistantMessages, o.Config.Caps.Seconds, o.Config.Caps.BudgetUSD),
	)
	if o.Profile.InspectSession {
		lines = append(lines, container.Line(docker, container.StateArgv(current.Box.SessionName()), false)+"   (the exit code and OOMKilled alone)")
	}
	lines = append(lines,
		container.Line(docker, current.Box.RemoveArgv(), false),
		container.Line(docker, current.Box.NetworkRemoveArgv(), false),
	)
	return Planned{Layout: l, Lines: lines, Step: step}
}

func quoted(words ...string) string {
	parts := make([]string, len(words))
	for index, word := range words {
		parts[index] = launch.Quote(word)
	}
	return strings.Join(parts, " ")
}
