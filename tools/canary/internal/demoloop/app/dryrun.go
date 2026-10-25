package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	canary "github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
)

// printPlan prints what a live run would do: the window, where the token would
// come from, and for each run the exact commands in order, with the run
// directory left as a placeholder. It reads no credential value.
func (r Runner) printPlan(o Options, now time.Time, open bool, source canary.ClaudeCredential, credentialErr error) error {
	state := "open"
	if !open {
		state = "closed (--force-window needed)"
	}
	fmt.Fprintf(r.Stdout, "now: %s, window %s: %s\n", now.Format("15:04"), r.Window.Describe(), state)
	switch {
	case credentialErr != nil:
		fmt.Fprintf(r.Stdout, "credentials: %v\n", credentialErr)
	case source == nil:
		fmt.Fprintf(r.Stdout, "credentials: none (store a token in the keychain as service %s, or set %s)\n", canary.KeychainService, launch.OAuthTokenVariable)
	default:
		fmt.Fprintf(r.Stdout, "credentials: source %s (%s=%s, given to the session only)\n", source.Source(), launch.OAuthTokenVariable, canary.TokenPlaceholder(source.Source()))
	}
	fmt.Fprintf(r.Stdout, "lock: %s\n", filepath.Join(o.RepoRoot, canary.LockRelative))
	runs := o.Config.Runs()
	fmt.Fprintf(r.Stdout, "runs, in order: %d\n", len(runs))
	for index, run := range runs {
		fmt.Fprintf(r.Stdout, "\nrun %d: %s (task %s, arm %s, repetition %d)\n", index+1, run.Name(), run.Task.Name, run.Arm, run.Repetition)
		for step, line := range planLines(o, run) {
			fmt.Fprintf(r.Stdout, "%2d. %s\n", step+1, line)
		}
	}
	fmt.Fprintln(r.Stdout, "\noutside the run directory a run writes: nothing but the lock; Claude Code runs with CLAUDE_CONFIG_DIR inside the run directory")
	return nil
}

// planLines are the commands of one run, in order.
func planLines(o Options, run config.Run) []string {
	l := plan.Layout{Repo: o.RepoRoot, Run: filepath.Join(o.OutDir, runDirPrefix+run.Name()+"-"+placeholderRun)}
	toolchain := make([]string, 0, len(o.Config.Toolchain))
	for _, tool := range o.Config.Toolchain {
		toolchain = append(toolchain, "<"+tool+" dir>")
	}
	leading := leadingDirs(l, toolchain, o.Claude)
	inRepo := "   (in " + o.RepoRoot + ")"
	inProject := "   (in " + l.Project() + ")"
	lines := []string{
		quoted(filepath.Join(o.RepoRoot, plan.MemGuardRelative), "check") + inRepo,
		quoted(o.Claude, versionArgument) + "   (must report " + o.ClaudePin + ")",
	}
	if len(o.Config.Toolchain) > 0 {
		lines = append(lines, quoted(append([]string{filepath.Join(o.RepoRoot, resolveToolDirs)}, o.Config.Toolchain...)...)+inRepo)
	}
	lines = append(lines, fmt.Sprintf("copy %s -> %s", filepath.Join(o.RepoRoot, run.Task.Fixture), l.Project()))
	steps := append(plan.BaseSteps(), plan.EquipSteps(l, run.Arm)...)
	for _, step := range steps {
		lines = append(lines, quoted(step.Argv...)+inProject)
	}
	if run.Arm.WithModules() && len(run.Task.Operations) > 0 {
		block, _ := plan.BindOperations(nil, run.Task.Operations)
		lines = append(lines, "append to "+filepath.Join(l.Project(), plan.SpecProofConfig)+":\n      "+strings.ReplaceAll(strings.TrimSpace(string(block)), "\n", "\n      "))
	}
	for _, step := range plan.CommitEquipSteps() {
		lines = append(lines, quoted(step.Argv...)+inProject)
	}
	lines = append(lines,
		quoted(plan.GateArgv(l, l.Record(record.InitialGateFile))...)+inProject,
		"write "+l.Path(plan.ClaudeConfigDir, launch.ClaudeGlobalConfigFile)+"   (the project trusted)",
		"write "+l.Path(plan.SessionSettings)+"   (permissions.allow: "+strings.Join(o.Config.AllowedTools, ", ")+")",
	)
	for _, name := range plan.Shimmed(run.Arm) {
		lines = append(lines, "write "+l.Path(plan.ShimDir, name)+"   (records the hook calls of "+l.Bin(name)+" in "+l.Record(record.HooksFile)+")")
	}
	command := plan.SessionCommand(plan.SessionInput{Layout: l, Run: run, Config: o.Config, Claude: o.Claude})
	variables := append(plan.Variables(l, leading), command.Env...)
	words := make([]string, 0, len(variables)+1)
	for _, variable := range variables {
		name, value, _ := strings.Cut(variable, "=")
		words = append(words, name+"="+launch.Quote(value))
	}
	words = append(words, launch.OAuthTokenVariable+"=<from the credential source>")
	lines = append(lines,
		"session environment: "+strings.Join(words, " "),
		launch.Command{Argv: command.Argv}.Line()+inProject,
		fmt.Sprintf("caps: %d assistant messages (the subagents' included), %d s, %s USD; the whole process group ends at a cap", o.Config.Caps.AssistantMessages, o.Config.Caps.Seconds, o.Config.Caps.BudgetUSD),
	)
	for _, argv := range plan.ChangeArgv() {
		lines = append(lines, quoted(argv...)+inProject)
	}
	lines = append(lines,
		"write "+l.Record(record.ChangeFile)+"   (what the git diff printed)",
		quoted(plan.GateArgv(l, l.Record(record.FinalGateFile))...)+inProject,
		"copy "+filepath.Join(l.Path(plan.CacheDir), "spec-proof", "*", record.MutantsDir)+" -> "+filepath.Join(l.Records(), record.MutantsDir),
		"write "+filepath.Join(l.Records(), record.CounterexamplesDir, "<mutant>"+record.DiffExtension)+"   (git diff --no-index of each mutant a gate block names)",
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
