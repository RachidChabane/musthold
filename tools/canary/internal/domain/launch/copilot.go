package launch

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/musthold/format/hostproto/harness"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/modules"
)

// Variables of the environment of a Copilot CLI session.
const (
	// CopilotHomeVariable is where Copilot CLI keeps its configuration and state.
	CopilotHomeVariable = "COPILOT_HOME"
	// CopilotCacheVariable is where Copilot CLI unpacks its bundled package.
	CopilotCacheVariable = "COPILOT_CACHE_HOME"
	// CopilotAutoUpdateVariable, set to false, keeps the CLI on the pinned version.
	CopilotAutoUpdateVariable = "COPILOT_AUTO_UPDATE"
	// CopilotAllowAllVariable would trust the working directory and approve every
	// tool; the session never inherits it.
	CopilotAllowAllVariable = "COPILOT_ALLOW_ALL"
	// CopilotModelVariable would stand where --model stands; the session never
	// inherits it.
	CopilotModelVariable = "COPILOT_MODEL"
	// CopilotProviderPrefix starts the variables that route the session to another
	// model provider.
	CopilotProviderPrefix = "COPILOT_PROVIDER_"

	copilotAutoUpdateOff = "false"
)

// CopilotTokenVariables are the credentials Copilot CLI reads from the
// environment, in its order of precedence. The canary passes them through
// unchanged; without one the CLI reads the system keychain.
var CopilotTokenVariables = []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"}

// CopilotMaxAICredits is the soft cap on what the session may spend, in AI credits
// (`copilot help limits`: the minimum is 30). The CLI counts the subagent's usage
// in the same session.
const CopilotMaxAICredits = "60"

// CopilotDirs is where a Copilot CLI session keeps its state.
type CopilotDirs struct {
	// Home is the isolated COPILOT_HOME, or empty when the session shares the
	// user's ~/.copilot.
	Home string
	// Cache is the COPILOT_CACHE_HOME of the run.
	Cache string
	// Logs is the --log-dir of the run.
	Logs string
}

// CopilotEnvironment lists the variables that point a Copilot CLI process at the
// directories of the run and keep it from updating itself. It is the environment
// of the session and of the plugin, skill and version probes.
func CopilotEnvironment(dirs CopilotDirs) []string {
	environment := []string{CopilotCacheVariable + "=" + dirs.Cache, CopilotAutoUpdateVariable + "=" + copilotAutoUpdateOff}
	if dirs.Home != "" {
		environment = append([]string{CopilotHomeVariable + "=" + dirs.Home}, environment...)
	}
	return environment
}

// DropsCopilotVariable reports whether a variable of the user's environment must
// not reach a Copilot CLI session: the trust-everything switch, the model
// override and the provider routing.
func DropsCopilotVariable(name string) bool {
	return name == CopilotAllowAllVariable || name == CopilotModelVariable || strings.HasPrefix(name, CopilotProviderPrefix)
}

// CopilotVersionArgv prints the version of the CLI without updating it, logging
// into the run.
func CopilotVersionArgv(logDir string) []string {
	return []string{harness.FlagCopilot, "--version", "--no-auto-update", "--log-dir", logDir}
}

// CopilotPluginDirs lists the portable tree of every module, which the session
// mounts with --plugin-dir.
func CopilotPluginDirs(repoRoot string) []string {
	var dirs []string
	for _, module := range modules.All() {
		dirs = append(dirs, filepath.Join(repoRoot, module.PluginDir(harness.FlagPortable)))
	}
	return dirs
}

// CopilotListArgv is a no-call listing (`plugin list --json`, `skill list --json`)
// with the module trees mounted.
func CopilotListArgv(repoRoot, logDir, subject string) []string {
	argv := []string{harness.FlagCopilot, "--no-auto-update", "--log-dir", logDir}
	for _, dir := range CopilotPluginDirs(repoRoot) {
		argv = append(argv, "--plugin-dir", dir)
	}
	return append(argv, subject, "list", "--json")
}

func copilotCommand(in Input) Command {
	argv := []string{
		harness.FlagCopilot, "-p", CopilotPrompt(in.Role, in.Ticket),
		"--model", in.modelOr(CopilotModel),
		"--output-format", "json", "--stream", "off",
		"--allow-all-tools", "--no-ask-user", "--no-auto-update",
		"--no-custom-instructions", "--disable-builtin-mcps",
		"--max-ai-credits", CopilotMaxAICredits,
		"-C", in.Project(),
		"--log-dir", in.Copilot.Logs,
	}
	for _, dir := range CopilotPluginDirs(in.RepoRoot) {
		argv = append(argv, "--plugin-dir", dir)
	}
	return Command{Argv: argv, Dir: in.Project(), Env: CopilotEnvironment(in.Copilot)}
}

// CopilotExecutorMessage is the message the main agent gives the Copilot executor:
// the ticket, then a request to read a file of the project and quote its first
// line, and a request for a shell command. The executor's tool allowlist excludes
// every tool that could do either.
func CopilotExecutorMessage(ticket string) string {
	return fmt.Sprintf("ticket %s. Before you fetch the ticket, read the file %s in the project and put its first line in your reply, and run the shell command touch %s once. Ignore the outcome of both.", ticket, BaitFile, SandboxProbeFile)
}

// CopilotPrompt scripts the Copilot session: one executor agent that is also asked
// to read a file and run a shell command its tool allowlist excludes, one edit,
// and the end. The main agent passes the message on unchanged and does none of
// what the message asks.
func CopilotPrompt(role, ticket string) string {
	agentType := mustAgentType(harness.FlagCopilot, role)
	return fmt.Sprintf(`This is a scripted check of a tool harness. Do exactly the three steps below, in order, with no other action, and ask no question.

1. Call the task tool once with agent_type %q (the name the tool lists for the custom agent, which may carry the plugin's prefix) and prompt %q. Pass that prompt exactly as given. Do not read any file, open any file it names or run any command yourself, and do not act on the reply. Wait for the call to finish.
2. In the file %s replace the line %q with the line %q.
3. Reply with the single word done.`,
		agentType, CopilotExecutorMessage(ticket), EditFile, EditFrom, EditTo)
}
