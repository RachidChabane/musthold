package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/modules"
)

// Locations, relative to the repository root, and names the run depends on.
const (
	toolsBinRelative = ".tools/bin"
	lockRelative     = ".tools/canary/session.lock"
	fixtureRelative  = "discpower/fixtures/prove/parcel"
	memGuardRelative = "script/mem-guard"
	shallnotBinary   = "shallnot"
	claudeBinary     = "claude"
	codexBinary      = "codex"

	shimDirName    = "shim"
	stateDirName   = "state"
	cacheDirName   = "cache"
	tmpDirName     = "tmp"
	claudeConfig   = "claude-config"
	hooksLogName   = evidence.HooksFile
	runDirPrefix   = "spec-proof-canary-"
	placeholderRun = "<run-dir>"

	oauthTokenVariable = "CLAUDE_CODE_OAUTH_TOKEN"

	proveTimeout   = 10 * time.Minute
	supportTimeout = 2 * time.Minute
	probeVersion   = "--version"
	exitAwaiting   = 3
)

// projectShallnotConfig is the Shallnot configuration of the fixture project: the
// requirements in specs, the pytest command whose JUnit file Shallnot reads, and
// the discriminating-power proof required of the code.
const projectShallnotConfig = `version: 1
specs:
  - specs
tests:
  - tests
results:
  - test-results/pytest.xml
test_commands:
  - pytest -o junit_family=xunit1 --junitxml=test-results/pytest.xml
proofs:
  trust: local
  code:
    - parcel/**
  require:
    - discriminating-power
`

// layout is where the parts of a run live.
type layout struct {
	repo, run                                      string
	project, state, cache, tmp, shim, records, bin string
	claudeConfig                                   string
}

func newLayout(repo, run string) layout {
	return layout{
		repo: repo, run: run,
		project: filepath.Join(run, launch.ProjectDir), state: filepath.Join(run, stateDirName),
		cache: filepath.Join(run, cacheDirName), tmp: filepath.Join(run, tmpDirName),
		shim: filepath.Join(run, shimDirName), records: filepath.Join(run, launch.RecordsDir),
		bin: filepath.Join(repo, toolsBinRelative), claudeConfig: filepath.Join(run, claudeConfig),
	}
}

func (l layout) hooksLog() string            { return filepath.Join(l.records, hooksLogName) }
func (l layout) binary(name string) string   { return filepath.Join(l.bin, name) }
func (l layout) shallnot() string            { return l.binary(shallnotBinary) }
func (l layout) fixture() string             { return filepath.Join(l.repo, fixtureRelative) }
func (l layout) record(name string) string   { return filepath.Join(l.records, name) }
func (l layout) primary() string             { return l.binary(modules.Primary().Binary) }
func (l layout) projectFile(n string) string { return filepath.Join(l.project, n) }

// environment returns the base environment with the variables set; a variable that
// is already there is replaced.
func environment(base []string, set ...string) []string {
	replaced := map[string]bool{}
	for _, pair := range set {
		name, _, _ := strings.Cut(pair, "=")
		replaced[name] = true
	}
	merged := make([]string, 0, len(base)+len(set))
	for _, pair := range base {
		name, _, _ := strings.Cut(pair, "=")
		if !replaced[name] {
			merged = append(merged, pair)
		}
	}
	return append(merged, set...)
}

func pathWith(base []string, dirs ...string) string {
	current := ""
	for _, pair := range base {
		if value, found := strings.CutPrefix(pair, "PATH="); found {
			current = value
		}
	}
	return "PATH=" + strings.Join(append(dirs, current), string(filepath.ListSeparator))
}

// supportEnvironment is the environment of the commands that prepare the project:
// the module binaries and Shallnot on the PATH, the state and caches of the run,
// and the project directory a harness exports to its hooks.
func (l layout) supportEnvironment(base []string) []string {
	return environment(base, l.sessionVariables(base, false)...)
}

// sessionEnvironment is the environment of the session: the recording shims first
// on the PATH.
func (l layout) sessionEnvironment(base []string, extra []string) []string {
	return environment(base, append(l.sessionVariables(base, true), extra...)...)
}

func (l layout) sessionVariables(base []string, shimmed bool) []string {
	dirs := []string{l.bin}
	if shimmed {
		dirs = []string{l.shim, l.bin}
	}
	variables := []string{
		pathWith(base, dirs...),
		"XDG_STATE_HOME=" + l.state, "XDG_CACHE_HOME=" + l.cache, "TMPDIR=" + l.tmp,
		"SPEC_PROOF_SHALLNOT=" + l.shallnot(), "GOMAXPROCS=2",
	}
	if !shimmed {
		variables = append(variables, "CLAUDE_PROJECT_DIR="+l.project)
	}
	return variables
}

// shimExports are the variables a shim sets before it hands over to the module.
func (l layout) shimExports() []string {
	exports := []string{"CLAUDE_PROJECT_DIR=" + l.project}
	for _, variable := range l.sessionVariables(nil, true) {
		if !strings.HasPrefix(variable, "PATH=") {
			exports = append(exports, variable)
		}
	}
	return exports
}

func (l layout) input(selection Selection, claudeConfigDir, codexHome string) launch.Input {
	return launch.Input{RepoRoot: l.repo, RunDir: l.run, Ticket: selection.Ticket, Role: selection.Role, ClaudeConfigDir: claudeConfigDir, CodexHome: codexHome}
}

// cliVersionArgv is the command that prints the version of a harness CLI.
func cliVersionArgv(flag string) []string { return []string{flag, probeVersion} }

func versionOfBinaryArgv(path string) []string { return []string{path, "version"} }

// proveArgv runs the primary module's proof until it asks the host.
func proveArgv(l layout) []string { return []string{l.primary(), "prove", "--all"} }

func listArgv(l layout) []string { return []string{l.primary(), "request", "list", "--json"} }

func initArgv(l layout, module modules.Module) []string {
	return []string{l.binary(module.Binary), "init", "--hooks", harness.FlagCodex}
}

func validateArgv(l layout, module modules.Module) []string {
	return []string{claudeBinary, "plugin", "validate", filepath.Join(l.repo, module.PluginDir(harness.FlagClaude))}
}

// Plan lists the exact command lines of a run for a harness, with the run
// directory and the ticket left as placeholders.
func Plan(flag, repo string, isolatedClaude bool, codexHome string) ([]string, error) {
	l := newLayout(repo, placeholderRun)
	claudeConfigDir := ""
	if isolatedClaude {
		claudeConfigDir = l.claudeConfig
	}
	placeholder := Selection{Ticket: "<ticket>", Role: hostproto.RoleFor(modules.Primary().Plugin, hostproto.AgentDecider)}
	session, err := launch.Build(flag, l.input(placeholder, claudeConfigDir, codexHome))
	if err != nil {
		return nil, err
	}
	lines := []string{
		strings.Join(launchQuote(filepath.Join(repo, memGuardRelative), "check"), " "),
		strings.Join(launchQuote(cliVersionArgv(flag)...), " "),
		fmt.Sprintf("copy %s -> %s", l.fixture(), l.project),
		"write " + l.projectFile("shallnot.yaml"),
	}
	if flag == harness.FlagCodex {
		for _, module := range modules.All() {
			lines = append(lines, strings.Join(launchQuote(initArgv(l, module)...), " ")+"   (in "+l.project+")")
		}
	}
	if flag == harness.FlagClaude {
		lines = append(lines, "write "+l.projectFile(launch.ClaudeSettingsPath))
		for _, module := range modules.All() {
			lines = append(lines, strings.Join(launchQuote(validateArgv(l, module)...), " "))
		}
	}
	lines = append(lines,
		strings.Join(launchQuote(proveArgv(l)...), " ")+"   (in "+l.project+", exit 3 expected)",
		strings.Join(launchQuote(listArgv(l)...), " "),
		strings.Join(launchQuote(l.primary(), "mcp", "--role", "executor", "--harness", flag), " ")+"   (stdio: fetch_request with no capability)",
		"session environment: "+strings.Join(sessionEnvironmentWords(l), " "),
		session.Line()+"   (in "+session.Dir+")",
	)
	return lines, nil
}

func launchQuote(words ...string) []string {
	quoted := make([]string, len(words))
	for index, word := range words {
		quoted[index] = launch.Quote(word)
	}
	return quoted
}

// sessionEnvironmentWords renders the variables the session adds to the
// environment of the canary, with the existing PATH kept at the end.
func sessionEnvironmentWords(l layout) []string {
	var words []string
	for _, variable := range l.sessionVariables(nil, true) {
		if value, isPath := strings.CutPrefix(variable, "PATH="); isPath {
			variable = "PATH=" + strings.TrimSuffix(value, string(filepath.ListSeparator)) + string(filepath.ListSeparator) + "$PATH"
			words = append(words, variable)
			continue
		}
		name, value, _ := strings.Cut(variable, "=")
		words = append(words, name+"="+launch.Quote(value))
	}
	return words
}
