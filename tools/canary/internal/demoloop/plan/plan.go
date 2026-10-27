// Package plan builds what one run of the agent loop executes: the layout of
// its directory, the commands that equip the project the documented way, the
// command line and environment of the Claude Code session, and the settings
// that bound it. It is pure: it runs and reads nothing, so the exact commands can
// be printed before anything starts.
package plan

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/format/hostproto/harness"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/modules"
)

// Directories of a run, relative to the run directory.
const (
	ProjectDir      = "project"
	RecordsDir      = "records"
	StateDir        = "state"
	CacheDir        = "cache"
	TmpDir          = "tmp"
	ShimDir         = "shim"
	ClaudeConfigDir = "claude-config"
	// HomeDir is the run's own HOME, for a layout that has one.
	HomeDir = "home"
	// SessionSettings is the settings file the session reads besides the project's.
	SessionSettings = "session-settings.json"
	// StopObjectsDir is the run's own git object store, which holds the project's
	// tree at each end-of-turn hook call; StopIndexFile is the index those trees
	// are written with, apart from the project's.
	StopObjectsDir = "objects"
	StopIndexFile  = "stop.index"
	// StopsRecordDir holds, in the records, one numbered directory per
	// end-of-turn hook call, with the gate's report and the project's tree.
	StopsRecordDir  = "stops"
	StopReportFile  = "report.json"
	StopTreeFile    = "tree.txt"
	stopCaptureFile = "stop-capture.log"
	// AcceptAsideDir holds, while the acceptance gate runs, the files of the
	// project it restores from the equipped commit.
	AcceptAsideDir = "accept-aside"
)

// Locations relative to the repository root.
const (
	ToolsBinRelative    = ".tools/bin"
	MemGuardRelative    = "script/mem-guard"
	ClaudePinRelative   = "script/claude-code.pin"
	ClaudeToolsRelative = ".tools/claude-code"
	ShallnotPinRelative = "script/shallnot.pin"
)

// Binaries the runner runs by name from the repository's tools directory.
const (
	ShallnotBinary = "shallnot"
	VerifyBinary   = "musthold-verify"
	ClaudeBinary   = "claude"
	GitBinary      = "git"
)

// Files of the project the equipping steps touch.
const (
	MustholdConfig = ".musthold/config.yaml"
	ShallnotConfig = "shallnot.yaml"
)

// Git identity and commits of the project: one author, one date and no user or
// system configuration, so that the base commits are the same on every machine.
const (
	BaseCommitTitle  = "base"
	EquipCommitTitle = "equip the project with Shallnot and the proof modules"
	commitDate       = "2026-01-01T00:00:00+00:00"
	commitName       = "demo"
	commitEmail      = "demo@example.invalid"
	initialBranch    = "main"
)

// Layout is where the parts of one run live.
type Layout struct {
	Repo, Run string
	// Product is the root of the product tree the run's commands see, as the
	// container mounts it: its binaries, the Claude Code CLI and the plugin
	// trees. Empty, the binaries are the repository's own.
	Product string
	// ClaudePath is the Claude Code CLI the session runs, as its container sees it;
	// empty, it is the product tree's.
	ClaudePath string
	// OwnHome gives the run a HOME of its own under the run directory, created
	// empty, in place of the containers' scratch HOME.
	OwnHome bool
}

// Parts of a product tree.
const (
	ProductBinDir     = "bin"
	ProductPluginsDir = "plugins"
	ProductClaudeDir  = "claude"
	// ProductUmbrella is the umbrella plugin among the plugin trees.
	ProductUmbrella = "musthold"
	// ProductShallnotPlugin is Shallnot's Claude Code plugin among the plugin
	// trees, staged from the pinned Shallnot source.
	ProductShallnotPlugin = "shallnot"
	// ProductRelayPlugin is the relay's Claude Code plugin among the plugin trees,
	// at its path in the repository.
	ProductRelayPlugin = "s1relay/plugin/claude"
)

// Names of the plugins a session loads besides the modules', as their manifests
// give them.
const (
	ShallnotPlugin = "shallnot"
	RelayPlugin    = "musthold-s1"
	UmbrellaPlugin = "musthold"
)

// Plugin is a Claude Code plugin tree a session loads, by the name its manifest
// gives it.
type Plugin struct {
	Name, Dir string
}

// Path joins a name to the run directory.
func (l Layout) Path(name ...string) string {
	return filepath.Join(append([]string{l.Run}, name...)...)
}

// Project is the project the session works in.
func (l Layout) Project() string { return l.Path(ProjectDir) }

// Records is where the run's evidence is written.
func (l Layout) Records() string { return l.Path(RecordsDir) }

// Record is one file of the records.
func (l Layout) Record(name string) string { return l.Path(RecordsDir, name) }

// Bin is a binary of the product tree, or of the repository's tools directory.
func (l Layout) Bin(name string) string {
	if l.Product != "" {
		return filepath.Join(l.Product, ProductBinDir, name)
	}
	return filepath.Join(l.Repo, ToolsBinRelative, name)
}

// BinDir is the directory of the binaries Bin names.
func (l Layout) BinDir() string { return filepath.Dir(l.Bin(ShallnotBinary)) }

// Claude is the Claude Code CLI the session runs: the layout's own, or the
// product tree's.
func (l Layout) Claude() string {
	if l.ClaudePath != "" {
		return l.ClaudePath
	}
	return filepath.Join(l.Product, ProductClaudeDir, ClaudeBinary)
}

// Home is the run's own HOME.
func (l Layout) Home() string { return l.Path(HomeDir) }

// Stops is the records' directory of the end-of-turn hook calls.
func (l Layout) Stops() string { return l.Record(StopsRecordDir) }

// pluginDir is a plugin tree of the product.
func (l Layout) pluginDir(relative string) string {
	return filepath.Join(l.Product, ProductPluginsDir, relative)
}

// Dirs lists the directories a run creates before it starts anything.
func (l Layout) Dirs() []string {
	dirs := []string{l.Path(StateDir), l.Path(CacheDir), l.Path(TmpDir), l.Path(ShimDir), l.Records(), l.Path(ClaudeConfigDir)}
	if l.OwnHome {
		dirs = append(dirs, l.Home())
	}
	return dirs
}

// Step is one command of the preparation, run in the project.
type Step struct {
	Argv []string
	// Purpose says why the step runs, for the plan and the records.
	Purpose string
}

// GitEnvironment is the environment every git command of the preparation adds.
func GitEnvironment() []string {
	return []string{
		"GIT_AUTHOR_NAME=" + commitName, "GIT_AUTHOR_EMAIL=" + commitEmail,
		"GIT_COMMITTER_NAME=" + commitName, "GIT_COMMITTER_EMAIL=" + commitEmail,
		"GIT_AUTHOR_DATE=" + commitDate, "GIT_COMMITTER_DATE=" + commitDate,
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	}
}

// BaseSteps put the copied fixture under version control as one commit.
func BaseSteps() []Step {
	return []Step{
		{Argv: []string{GitBinary, "init", "--quiet", "--initial-branch", initialBranch}, Purpose: "put the project under version control"},
		{Argv: []string{GitBinary, "add", "--all"}, Purpose: "stage the fixture"},
		{Argv: []string{GitBinary, "commit", "--quiet", "--message", BaseCommitTitle}, Purpose: "commit the fixture"},
	}
}

// EquipSteps equip the project for an arm the documented way: `shallnot init`,
// then `init` and `enable` of each proof module the arm installs.
func EquipSteps(l Layout, arm config.Arm) []Step {
	steps := []Step{{Argv: []string{l.Bin(ShallnotBinary), "init"}, Purpose: "install Shallnot and its end-of-turn hook"}}
	if !arm.WithModules() {
		return steps
	}
	for _, module := range modules.All() {
		steps = append(steps,
			Step{Argv: []string{l.Bin(module.Binary), "init"}, Purpose: "install " + module.Binary},
			Step{Argv: []string{l.Bin(module.Binary), "enable"}, Purpose: "turn on the " + module.Subject + " proof"},
		)
	}
	return steps
}

// CommitEquipSteps commit what equipping wrote, so that the agent's change is
// what the project holds beyond that commit.
func CommitEquipSteps() []Step {
	return []Step{
		{Argv: []string{GitBinary, "add", "--all"}, Purpose: "stage what equipping wrote"},
		{Argv: []string{GitBinary, "commit", "--quiet", "--message", EquipCommitTitle}, Purpose: "commit the equipped project"},
	}
}

// GateArgv runs the gate and writes its report as JSON.
func GateArgv(l Layout, report string) []string {
	return []string{l.Bin(ShallnotBinary), "gate", "--json-out", report}
}

// ChangeArgv are the commands that record what the session changed since the
// equipped commit, untracked files included.
func ChangeArgv() [][]string {
	return [][]string{
		{GitBinary, "add", "--all"},
		{GitBinary, "diff", "--cached", "--no-color", "--no-ext-diff", "HEAD"},
	}
}

// ErrOperationsBound is returned for a configuration that already binds operations.
var ErrOperationsBound = fmt.Errorf("%s already has an operations block", MustholdConfig)

// BindOperations appends the task's operations to the shared configuration of the
// modules, the step getting started documents after `init`. The block lists the
// operations and their languages in alphabetical order.
func BindOperations(current []byte, operations map[string]map[string]string) ([]byte, error) {
	for _, line := range strings.Split(string(current), "\n") {
		if strings.HasPrefix(line, "operations:") {
			return nil, ErrOperationsBound
		}
	}
	var block strings.Builder
	if len(current) > 0 && !strings.HasSuffix(string(current), "\n") {
		block.WriteString("\n")
	}
	block.WriteString("operations:\n")
	for _, operation := range sortedKeys(operations) {
		fmt.Fprintf(&block, "  %s:\n", operation)
		targets := operations[operation]
		for _, language := range sortedKeys(targets) {
			fmt.Fprintf(&block, "    %s: %s\n", language, quoted(targets[language]))
		}
	}
	return append(append([]byte(nil), current...), block.String()...), nil
}

func sortedKeys[V any](entries map[string]V) []string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func quoted(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// Plugins are the plugin trees of the product the session loads for an arm: each
// proof module's Claude Code plugin, then the plugins the umbrella depends on
// besides them, Shallnot's and the relay's, then the umbrella plugin that holds
// `prove-all`. The traceability arm loads none: Shallnot's hook and skills come
// from the files `shallnot init` writes, and Shallnot's plugin runs its end-of-turn
// hook only in a project that does not carry that hook already.
func Plugins(l Layout, arm config.Arm) []Plugin {
	if !arm.WithModules() {
		return nil
	}
	var plugins []Plugin
	for _, module := range modules.All() {
		plugins = append(plugins, Plugin{Name: module.Plugin, Dir: l.pluginDir(module.PluginDir(harness.FlagClaude))})
	}
	return append(plugins,
		Plugin{Name: ShallnotPlugin, Dir: l.pluginDir(ProductShallnotPlugin)},
		Plugin{Name: RelayPlugin, Dir: l.pluginDir(ProductRelayPlugin)},
		Plugin{Name: UmbrellaPlugin, Dir: l.pluginDir(ProductUmbrella)},
	)
}

// PluginNames are the names of the plugins the session of an arm must load.
func PluginNames(arm config.Arm) []string {
	var names []string
	for _, plugin := range Plugins(Layout{}, arm) {
		names = append(names, plugin.Name)
	}
	return names
}

// Shimmed lists the binaries whose hook calls the session records: Shallnot,
// whose end-of-turn hook is the gate, and each module the arm installs.
func Shimmed(arm config.Arm) []string {
	names := []string{ShallnotBinary}
	if arm.WithModules() {
		for _, module := range modules.All() {
			names = append(names, module.Binary)
		}
	}
	return names
}

// The tools of the session that reach the web.
const (
	WebFetchTool  = "WebFetch"
	WebSearchTool = "WebSearch"
)

// WebTools are the tools a session that must reach no host but the model's is
// denied.
func WebTools() []string { return []string{WebFetchTool, WebSearchTool} }

// permissions is the `permissions` block of the session settings.
type permissions struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny,omitempty"`
}

// SessionSettingsJSON is the settings file the session reads on top of the
// project's: the permission rules that allow, those that deny when any are
// given, and nothing else.
func SessionSettingsJSON(allow, deny []string) ([]byte, error) {
	settings := map[string]permissions{"permissions": {Allow: append([]string{}, allow...), Deny: append([]string(nil), deny...)}}
	encoded, err := json.MarshalIndent(settings, "", "  ")
	return append(encoded, '\n'), err
}

// SessionInput is everything the session's command line depends on.
type SessionInput struct {
	Layout Layout
	Run    config.Run
	Config config.Config
	// Claude is the path of the pinned Claude Code binary.
	Claude string
}

// SessionCommand is the Claude Code session of a run: one non-interactive turn
// on the task's prompt, its events streamed as JSON with the hook events, the
// project's settings and the session settings only, edits accepted, the spend
// capped, and the arm's plugins loaded from their trees.
func SessionCommand(in SessionInput) launch.Command {
	argv := []string{
		in.Claude, "-p", in.Run.Task.Prompt,
		"--model", in.Config.Model,
		"--output-format", "stream-json", "--verbose",
		"--include-hook-events", "--forward-subagent-text",
		"--setting-sources", "project",
		"--settings", in.Layout.Path(SessionSettings),
		"--permission-mode", "acceptEdits",
		"--max-budget-usd", in.Config.Caps.BudgetUSD,
	}
	for _, plugin := range Plugins(in.Layout, in.Run.Arm) {
		argv = append(argv, "--plugin-dir", plugin.Dir)
	}
	return launch.Command{Argv: argv, Dir: in.Layout.Project(), Env: []string{"CLAUDE_CONFIG_DIR=" + in.Layout.Path(ClaudeConfigDir)}}
}

// Variables the run sets in the environment of everything it starts.
const (
	pathVariable = "PATH"
	// NoBytecode keeps Python from writing caches into the project, whose files
	// the proofs fingerprint.
	NoBytecode = "PYTHONDONTWRITEBYTECODE=1"
	// NoUpdater keeps the pinned CLI from replacing itself.
	NoUpdater = "DISABLE_AUTOUPDATER=1"
	// FewThreads bounds the threads of the Go binaries.
	FewThreads = "GOMAXPROCS=2"
)

// Variables are the variables the run adds to the environment it inherits: the
// PATH led by the given directories, the state, caches and temporary files of the
// run, the Shallnot the modules check, and the switches that keep the machine and
// the project quiet. Shallnot keeps its count of the hook's blocks in the
// temporary directory, so each run counts its own.
func Variables(l Layout, leading []string) []string {
	return []string{
		pathVariable + "=" + strings.Join(leading, string(filepath.ListSeparator)) + string(filepath.ListSeparator) + "$PATH",
		"XDG_STATE_HOME=" + l.Path(StateDir), "XDG_CACHE_HOME=" + l.Path(CacheDir), "TMPDIR=" + l.Path(TmpDir),
		"MUSTHOLD_SHALLNOT=" + l.Bin(ShallnotBinary), FewThreads, NoBytecode, NoUpdater,
	}
}

// Environment resolves Variables against the inherited environment: `$PATH`
// becomes the inherited PATH, and a variable already set is replaced.
func Environment(base []string, variables []string) []string {
	inherited := ""
	for _, pair := range base {
		if value, found := strings.CutPrefix(pair, pathVariable+"="); found {
			inherited = value
		}
	}
	resolved := make([]string, len(variables))
	replaced := map[string]bool{}
	for index, variable := range variables {
		name, value, _ := strings.Cut(variable, "=")
		if name == pathVariable {
			value = strings.TrimSuffix(strings.Replace(value, "$PATH", inherited, 1), string(filepath.ListSeparator))
		}
		resolved[index] = name + "=" + value
		replaced[name] = true
	}
	merged := make([]string, 0, len(base)+len(resolved))
	for _, pair := range launch.WithoutOAuthToken(base) {
		name, _, _ := strings.Cut(pair, "=")
		if !replaced[name] {
			merged = append(merged, pair)
		}
	}
	return append(merged, resolved...)
}

// ProjectDirVariable names the project to a hook or an executor server that a
// harness starts with a reduced environment.
const ProjectDirVariable = "CLAUDE_PROJECT_DIR"

// ShimExports are the variables a shim sets: the project directory, then the
// run's variables.
func ShimExports(l Layout, variables []string) []string {
	return append([]string{ProjectDirVariable + "=" + l.Project()}, Environment(nil, variables)...)
}

// ShimScript is the executable that stands in for a binary on the session's
// PATH: it exports the run's variables, because a harness may start a hook with a
// reduced environment, and hands over to the runner's shim, which records the
// hook calls of the real binary.
func ShimScript(runner, real, hooksLog string, exports []string) string {
	var script strings.Builder
	writeShimExports(&script, exports)
	fmt.Fprintf(&script, "exec %s\n", shimCall(runner, real, hooksLog))
	return script.String()
}

func writeShimExports(script *strings.Builder, exports []string) {
	script.WriteString("#!/bin/sh\n")
	for _, variable := range exports {
		name, value, _ := strings.Cut(variable, "=")
		if name == pathVariable {
			continue
		}
		fmt.Fprintf(script, "export %s=%s\n", name, launch.Quote(value))
	}
}

// shimCall hands the arguments over to the runner's shim.
func shimCall(runner, real, hooksLog string) string {
	return fmt.Sprintf("%s %s %s %s %s %s -- \"$@\"",
		launch.Quote(runner), launch.ShimSubcommand, launch.FlagReal, launch.Quote(real), launch.FlagRecord, launch.Quote(hooksLog))
}

// The arguments of Shallnot's end-of-turn hook as a Claude Code session calls it.
const (
	hookArgument      = "hook"
	claudeStopHook    = "claude-stop"
	checkSubcommand   = "check"
	jsonOutFlag       = "--json-out"
	gitIndexVariable  = "GIT_INDEX_FILE"
	gitObjectsVar     = "GIT_OBJECT_DIRECTORY"
	gitAlternatesVar  = "GIT_ALTERNATE_OBJECT_DIRECTORIES"
	projectObjectsRel = ".git/objects"
)

// StopShimScript is the shim of Shallnot for a run that records each end-of-turn
// hook call: once the hook has returned, it numbers the call from 1, writes
// Shallnot's JSON report on the same files with `check --json-out`, and writes
// the project's tree id into the run's own object store through an index of its
// own, so that neither the project's index nor its work tree changes. What the
// capture prints goes to a log of the run, never to the harness, and the shim
// exits with the hook's exit code.
func StopShimScript(runner, real, hooksLog string, exports []string, l Layout) string {
	var script strings.Builder
	writeShimExports(&script, exports)
	stops, project := launch.Quote(l.Stops()), launch.Quote(l.Project())
	fmt.Fprintf(&script, "%s\nstatus=$?\n", shimCall(runner, real, hooksLog))
	fmt.Fprintf(&script, "if [ \"${1:-}\" = %s ] && [ \"${2:-}\" = %s ]; then\n{\n", hookArgument, claudeStopHook)
	fmt.Fprintf(&script, "  mkdir -p %s %s\n", stops, launch.Quote(l.Path(StopObjectsDir)))
	fmt.Fprintf(&script, "  n=1\n  while [ -e %s/$n ]; do n=$((n + 1)); done\n  mkdir %s/$n\n", stops, stops)
	fmt.Fprintf(&script, "  cd %s\n", project)
	fmt.Fprintf(&script, "  %s %s %s %s/$n/%s\n", launch.Quote(real), checkSubcommand, jsonOutFlag, stops, StopReportFile)
	fmt.Fprintf(&script, "  rm -f %s\n", launch.Quote(l.Path(StopIndexFile)))
	fmt.Fprintf(&script, "  export %s=%s %s=%s %s=%s\n",
		gitIndexVariable, launch.Quote(l.Path(StopIndexFile)),
		gitObjectsVar, launch.Quote(l.Path(StopObjectsDir)),
		gitAlternatesVar, launch.Quote(filepath.Join(l.Project(), projectObjectsRel)))
	for _, variable := range container.TrustedRepository(l.Project()) {
		name, value, _ := strings.Cut(variable, "=")
		fmt.Fprintf(&script, "  export %s=%s\n", name, launch.Quote(value))
	}
	fmt.Fprintf(&script, "  %s add --all && %s write-tree > %s/$n/%s\n", GitBinary, GitBinary, stops, StopTreeFile)
	fmt.Fprintf(&script, "} >>%s 2>&1 </dev/null\nfi\nexit $status\n", launch.Quote(l.Record(stopCaptureFile)))
	return script.String()
}

// The files the acceptance gate restores from the equipped commit, and the shell
// that moves them.
var acceptRestored = []string{ShallnotConfig, "specs", "AGENTS.md"}

const shellBinary = "sh"

// movedMarker, in the aside directory, says that every file to restore was moved
// aside: a file absent there was absent from the project the session left.
const movedMarker = ".moved"

// Acceptance runs the acceptance gate on the project a session left. Restore
// moves the session's Shallnot configuration, specs and AGENTS.md aside and writes
// those of the equipped commit in their place; Gate is the gate; PutBack puts the
// session's own back, so that the project is again what the session left. PutBack
// runs whatever Restore and Gate did, and changes nothing when Restore moved
// nothing.
type Acceptance struct {
	Restore, Gate, PutBack Step
}

// Steps lists the acceptance gate's steps in order.
func (a Acceptance) Steps() []Step { return []Step{a.Restore, a.Gate, a.PutBack} }

// AcceptanceSteps are the steps of the acceptance gate against the equipped
// commit, writing the gate's report.
func AcceptanceSteps(l Layout, equipped, report string) Acceptance {
	aside := l.Path(AcceptAsideDir)
	paths := strings.Join(acceptRestored, " ")
	restore := "set -eu\nmkdir -p \"$1\"\nfor p in " + paths + "; do if [ -e \"$p\" ] || [ -L \"$p\" ]; then mv \"$p\" \"$1/\"; fi; done\n" +
		": > \"$1/" + movedMarker + "\"\n" +
		"kept=$(" + GitBinary + " ls-tree --name-only \"$2\" -- " + paths + ")\n" +
		"if [ -n \"$kept\" ]; then " + GitBinary + " archive --format=tar \"$2\" -- $kept | tar -xf -; fi\n"
	putBack := "set -eu\n[ -d \"$1\" ] || exit 0\nfor p in " + paths + "; do\n" +
		"  if [ -e \"$1/$p\" ] || [ -L \"$1/$p\" ]; then rm -rf \"$p\"; mv \"$1/$p\" \"$p\";\n" +
		"  elif [ -e \"$1/" + movedMarker + "\" ]; then rm -rf \"$p\"; fi\n" +
		"done\nrm -f \"$1/" + movedMarker + "\"\nrmdir \"$1\"\n"
	return Acceptance{
		Restore: Step{Argv: []string{shellBinary, "-c", restore, shellBinary, aside, equipped}, Purpose: "restore shallnot.yaml, specs/ and AGENTS.md from the equipped commit"},
		Gate:    Step{Argv: GateArgv(l, report), Purpose: "run the acceptance gate on the project the session left"},
		PutBack: Step{Argv: []string{shellBinary, "-c", putBack, shellBinary, aside}, Purpose: "put back the session's shallnot.yaml, specs/ and AGENTS.md"},
	}
}

// BaseCommitArgv names the base commit of a prepared project, its root commit.
func BaseCommitArgv() []string { return []string{GitBinary, "rev-list", "--max-parents=0", "HEAD"} }

// HeadCommitArgv names the commit the project is at.
func HeadCommitArgv() []string { return []string{GitBinary, "rev-parse", "HEAD"} }

// EquippedFilesArgv lists, NUL-separated, the files the equipped commit adds or
// changes relative to the base commit.
func EquippedFilesArgv(base string) []string {
	return []string{GitBinary, "diff", "--name-only", "-z", "--no-renames", "--diff-filter=AM", base, "HEAD"}
}

// TrackedFilesArgv lists the files of the commit the project is at, NUL-separated.
func TrackedFilesArgv() []string { return []string{GitBinary, "ls-files", "-z"} }
