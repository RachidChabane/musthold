package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	canary "github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
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
		filepath.Join(o.RepoRoot, canary.LockRelative), o.Work, o.Product, o.Image, strings.Join(container.ModelEndpoints(), ", "))
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
	current := r.newRun(o, dir)
	l := current.layout
	docker := strings.Join(o.Docker, " ")
	step := func(argv []string) string {
		current.steps.count++
		env := plan.Environment(containerBase(l), plan.Variables(l, leadingDirs(l)))
		if argv[0] == plan.GitBinary {
			env = append(env, plan.GitEnvironment()...)
		}
		return container.Line(docker, current.box.StepArgv(current.steps.count, l.Project(), env, argv), false)
	}
	lines := []string{
		quoted(filepath.Join(o.RepoRoot, plan.MemGuardRelative), "check") + "   (on the host)",
		"sha256 " + filepath.Join(o.Product, plan.ProductClaudeDir, plan.ClaudeBinary) + "   (recorded in run.json)",
		step([]string{l.Claude(), versionArgument}) + "   (must report " + o.ClaudePin + ")",
		fmt.Sprintf("copy %s -> %s   (on the host)", filepath.Join(o.RepoRoot, configured.Task.Fixture), l.Project()),
	}
	for _, planned := range append(plan.BaseSteps(), plan.EquipSteps(l, configured.Arm)...) {
		lines = append(lines, step(planned.Argv))
	}
	if configured.Arm.WithModules() && len(configured.Task.Operations) > 0 {
		block, _ := plan.BindOperations(nil, configured.Task.Operations)
		lines = append(lines, "append to "+filepath.Join(l.Project(), plan.SpecProofConfig)+":\n      "+strings.ReplaceAll(strings.TrimSpace(string(block)), "\n", "\n      "))
	}
	for _, planned := range plan.CommitEquipSteps() {
		lines = append(lines, step(planned.Argv))
	}
	lines = append(lines,
		step(plan.GateArgv(l, l.Record(record.InitialGateFile))),
		"write "+l.Path(plan.ClaudeConfigDir, launch.ClaudeGlobalConfigFile)+"   (the project trusted)",
		"write "+l.Path(plan.SessionSettings)+"   (permissions.allow: "+strings.Join(o.Config.AllowedTools, ", ")+")",
	)
	for _, name := range plan.Shimmed(configured.Arm) {
		lines = append(lines, "write "+l.Path(plan.ShimDir, name)+"   (records the hook calls of "+l.Bin(name)+" in "+l.Record(record.HooksFile)+")")
	}
	session := sessionOf(o, l, current.box, configured)
	lines = append(lines,
		container.Line(docker, current.box.NetworkArgv(), false)+"   (no route out)",
		container.Line(docker, current.box.ProxyArgv(l.Record(record.EgressFile)), false),
		container.Line(docker, current.box.ConnectArgv(), false),
		container.Line(session.argv[0], session.argv[1:], true),
		fmt.Sprintf("caps: %d assistant messages (the subagents' included), %d s, %s USD; the docker client's process group ends at a cap",
			o.Config.Caps.AssistantMessages, o.Config.Caps.Seconds, o.Config.Caps.BudgetUSD),
		container.Line(docker, current.box.RemoveArgv(), false),
		container.Line(docker, current.box.NetworkRemoveArgv(), false),
	)
	for _, argv := range plan.ChangeArgv() {
		lines = append(lines, step(argv))
	}
	lines = append(lines,
		"write "+l.Record(record.ChangeFile)+"   (what the git diff printed)",
		step(plan.GateArgv(l, l.Record(record.FinalGateFile))),
		"scan every record for the token and for any text of a token's shape; a hit fails the run",
		"write "+l.Record(record.TranscriptMarkdown)+" and "+l.Record(record.TranscriptHTML),
	)
	return lines
}

func quoted(words ...string) string {
	parts := make([]string, len(words))
	for index, word := range words {
		parts[index] = launch.Quote(word)
	}
	return strings.Join(parts, " ")
}
