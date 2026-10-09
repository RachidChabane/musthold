// Package launch builds what a canary session runs: the command line of the
// harness CLI, the environment around it, the prompt that scripts the session and
// the project-local settings that record it. It is pure: it runs nothing and
// reads nothing, so the exact command lines can be printed before anything is
// started.
package launch

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/modules"
)

// Models the sessions run on.
const (
	ClaudeModel = "claude-sonnet-5-5"
	CodexModel  = "gpt-6-luna"
	// CopilotModel is the default model of the Copilot session, in the spelling
	// `copilot help config` lists: the default of the CLI itself, which every
	// account that can run it may use.
	CopilotModel = "gpt-5.6-luna"
)

// Caps that bound a session. The runner ends the session when one is reached.
const (
	// ClaudeEventCap bounds the assistant messages of a session, the subagent's included.
	ClaudeEventCap = 40
	// CodexEventCap bounds the completed items that stand for a model step.
	CodexEventCap = 40
	// CopilotEventCap bounds the complete assistant messages and the tool starts of
	// a session, the subagent's included.
	CopilotEventCap = 40
	// TimeoutSeconds bounds the wall-clock time of a session.
	TimeoutSeconds = 900
	// ClaudeBudgetUSD bounds what Claude Code may spend on the session.
	ClaudeBudgetUSD = "1.50"
	// CodexReasoningEffort keeps the Codex session short.
	CodexReasoningEffort = "low"
)

// The one edit the session makes to the project.
const (
	EditFile = "parcel/rates.py"
	EditFrom = "SHIPPING_FEE = 4.5"
	EditTo   = "SHIPPING_FEE = 4.50"
	// SandboxProbeFile is the file an executor that may not use a shell is asked to
	// create.
	SandboxProbeFile = "sandbox-probe.txt"
	// BaitFile is the file of the project an executor that may not read files is
	// asked to read. It holds a token the canary generates for the run.
	BaitFile = "canary-note.txt"
)

// Directories of a run, relative to the run directory.
const (
	ProjectDir = "project"
	RecordsDir = "records"
	// BodiesDir holds the raw API log of a Claude Code session.
	BodiesDir = "bodies"
)

// The module the classifier check of a Claude Code session runs, and its commands.
const (
	// ClassifyModuleBinary is the proof module whose requests the mod classifies in
	// shadow; the switch names no other module.
	ClassifyModuleBinary = "spec-proof-completeness"
	proveCommand         = "prove --all"
	resumeCommand        = "prove --resume"
)

// RawLogEnvironment is what makes Claude Code write the raw API log of a session
// into the directory: the telemetry switch the file mode needs, no exporter, and
// the log itself.
func RawLogEnvironment(bodies string) []string {
	return []string{
		"CLAUDE_CODE_ENABLE_TELEMETRY=1", "OTEL_LOGS_EXPORTER=none", "OTEL_METRICS_EXPORTER=none",
		"OTEL_LOG_RAW_API_BODIES=file:" + bodies,
	}
}

// ClassifyConfig adds to the configuration of the fixture project what the
// classifier check needs: the completeness proof, the classification of its
// requests in shadow and of no other module's, and the directory of the raw API
// log.
func ClassifyConfig(fixture, bodies string) string {
	enabled := strings.Replace(fixture, "completeness: false", "completeness: true", 1)
	if !strings.HasSuffix(enabled, "\n") {
		enabled += "\n"
	}
	return enabled + "systemone:\n  classify:\n    default: off\n    completeness: shadow\nhost:\n  classify:\n    witness_dir: " + bodies + "\n"
}

// ErrUnknownHarness is returned for a harness the canary cannot launch.
var ErrUnknownHarness = errors.New("no launch for the harness")

// Command is a process to run.
type Command struct {
	Argv []string
	Dir  string
	// Env holds the variables added to the environment of the canary.
	Env []string
}

// Line renders the command as one shell line, the environment first.
func (c Command) Line() string {
	parts := make([]string, 0, len(c.Env)+len(c.Argv))
	for _, variable := range c.Env {
		name, value, _ := strings.Cut(variable, "=")
		parts = append(parts, name+"="+Quote(value))
	}
	for _, arg := range c.Argv {
		parts = append(parts, Quote(arg))
	}
	return strings.Join(parts, " ")
}

// Quote quotes a word for a POSIX shell unless it is plainly safe.
func Quote(word string) string {
	if word != "" && strings.IndexFunc(word, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./=:,@%+", r))
	}) < 0 {
		return word
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// Input is everything the command of a session depends on.
type Input struct {
	// RepoRoot is the repository the plugin trees come from.
	RepoRoot string
	// RunDir is the directory of this run.
	RunDir string
	Ticket string
	Role   string
	// ClaudeConfigDir is the isolated configuration directory of Claude Code, or
	// empty when the session shares the user's.
	ClaudeConfigDir string
	// CodexHome is a dedicated Codex home, or empty to use the user's.
	CodexHome string
	// Model is the model the operator chose for the session, or empty for the
	// harness's default.
	Model string
	// Copilot says where the Copilot CLI keeps its state.
	Copilot CopilotDirs
}

func (i Input) modelOr(fallback string) string {
	if i.Model != "" {
		return i.Model
	}
	return fallback
}

// Bodies is where the raw API log of the session is written.
func (i Input) Bodies() string { return filepath.Join(i.RunDir, BodiesDir) }

// Project is the temporary project the session works in.
func (i Input) Project() string { return filepath.Join(i.RunDir, ProjectDir) }

// Records is where the session's evidence is written.
func (i Input) Records() string { return filepath.Join(i.RunDir, RecordsDir) }

// Build returns the command of a harness's session.
func Build(flag string, in Input) (Command, error) {
	switch flag {
	case harness.FlagClaude:
		return claudeCommand(in), nil
	case harness.FlagCodex:
		return codexCommand(in), nil
	case harness.FlagCopilot:
		return copilotCommand(in), nil
	default:
		return Command{}, fmt.Errorf("%w %q", ErrUnknownHarness, flag)
	}
}

func claudeCommand(in Input) Command {
	argv := []string{
		"claude", "-p", ClaudePrompt(in.Role, in.Ticket),
		"--model", in.modelOr(ClaudeModel),
		"--output-format", "stream-json", "--verbose",
		"--include-hook-events", "--forward-subagent-text",
		"--setting-sources", "project",
		"--permission-mode", "acceptEdits",
		"--max-budget-usd", ClaudeBudgetUSD,
	}
	for _, module := range modules.All() {
		argv = append(argv, "--plugin-dir", filepath.Join(in.RepoRoot, module.PluginDir(harness.FlagClaude)))
	}
	command := Command{Argv: argv, Dir: in.Project(), Env: RawLogEnvironment(in.Bodies())}
	if in.ClaudeConfigDir != "" {
		command.Env = append(command.Env, "CLAUDE_CONFIG_DIR="+in.ClaudeConfigDir)
	}
	return command
}

func codexCommand(in Input) Command {
	argv := []string{
		"codex", "exec", "-m", in.modelOr(CodexModel), "--json",
		"--skip-git-repo-check", "--ignore-rules",
		"--enable", "hooks", "--enable", "multi_agent",
		"--dangerously-bypass-hook-trust",
		"-s", "workspace-write",
		"-C", in.Project(),
		"-c", fmt.Sprintf("projects.%q.trust_level=%q", in.Project(), "trusted"),
		"-c", "model_reasoning_effort=" + CodexReasoningEffort,
		"-c", "web_search=disabled",
		CodexPrompt(in.Role, in.Ticket),
	}
	command := Command{Argv: argv, Dir: in.Project()}
	if in.CodexHome != "" {
		command.Env = append(command.Env, "CODEX_HOME="+in.CodexHome)
	}
	return command
}

// PaddedLaunchPrompt is the prompt the main agent is told to give the executor: the
// ticket among words the launch hook must remove.
func PaddedLaunchPrompt(ticket string) string {
	const padding = "Mind the boundary at 50."
	return padding + " ticket " + ticket + " " + padding
}

// ClaudePrompt scripts the Claude Code session: a fetch the main session must be
// refused, one executor launch the hook must rewrite, one edit, a run and a resume
// of the completeness proof whose requests the mod classifies, and the end.
func ClaudePrompt(role, ticket string) string {
	fetchTool := mustTool(harness.FlagClaude)
	return fmt.Sprintf(`This is a scripted check of a tool harness. Do exactly the six steps below, in order, with no other tool call, and ask no question.

1. Call the tool %s once with the argument {"ticket": %q}. The call is expected to be refused. Do not retry it and do not work around the refusal.
2. Launch exactly one subagent with the Agent tool, in the foreground: subagent_type %q, model "haiku", description "canary run", prompt %q. Wait for it to finish and do not act on its reply.
3. In the file %s replace the line %q with the line %q.
4. Run the shell command %q once and ignore its outcome.
5. Run the shell command %q once and ignore its outcome.
6. Reply with the single word done.`,
		fetchTool, ticket, role, PaddedLaunchPrompt(ticket), EditFile, EditFrom, EditTo,
		ClassifyModuleBinary+" "+proveCommand, ClassifyModuleBinary+" "+resumeCommand)
}

// ExecutorMessage is the message the main agent gives the executor: the ticket and
// the request for a write that the executor's read-only sandbox must refuse.
func ExecutorMessage(ticket string) string {
	return fmt.Sprintf("ticket %s. Before you fetch the ticket, run the shell command touch %s once and ignore its outcome.", ticket, SandboxProbeFile)
}

// CodexPrompt scripts the Codex session: one executor spawn that also asks the
// executor for a write the read-only sandbox must refuse, one edit, and the end.
func CodexPrompt(role, ticket string) string {
	agentType := mustAgentType(harness.FlagCodex, role)
	return fmt.Sprintf(`This is a scripted check of a tool harness. Do exactly the three steps below, in order, with no other action, and ask no question.

1. Call spawn_agent with agent_type %q, fork_turns "none" and message %q, then call wait_agent for it. Do not act on its reply.
2. In the file %s replace the line %q with the line %q.
3. Reply with the single word done.`,
		agentType, ExecutorMessage(ticket), EditFile, EditFrom, EditTo)
}

func mustTool(flag string) string {
	selected, err := harness.ByFlag(modules.Primary().Identity(), flag)
	if err != nil {
		panic(err)
	}
	return selected.ToolNames()[0]
}

func mustAgentType(flag, role string) string {
	selected, err := harness.ByFlag(modules.Primary().Identity(), flag)
	if err != nil {
		panic(err)
	}
	return selected.AgentType(role)
}

// CountsAsEvent reports whether a line of the session's event stream is one of
// the model steps the event cap counts.
func CountsAsEvent(flag string, line []byte) bool {
	var event struct {
		Type string `json:"type"`
		Item struct {
			Type string `json:"type"`
		} `json:"item"`
	}
	if json.Unmarshal(line, &event) != nil {
		return false
	}
	switch flag {
	case harness.FlagClaude:
		return event.Type == evidence.ClaudeTypeAssistant
	case harness.FlagCodex:
		return event.Type == evidence.CodexEventItemCompleted && slices.Contains(
			[]string{evidence.CodexItemMessage, evidence.CodexItemCommand, evidence.CodexItemMCPCall, evidence.CodexItemCollab}, event.Item.Type)
	case harness.FlagCopilot:
		return slices.Contains([]string{evidence.CopilotEventAssistantMessage, evidence.CopilotEventToolStart}, event.Type)
	default:
		return false
	}
}

// EventCap is the cap of a harness.
func EventCap(flag string) int {
	switch flag {
	case harness.FlagCodex:
		return CodexEventCap
	case harness.FlagCopilot:
		return CopilotEventCap
	default:
		return ClaudeEventCap
	}
}

// DefaultModel is the model a harness's session runs on unless the operator names
// another.
func DefaultModel(flag string) string {
	switch flag {
	case harness.FlagCodex:
		return CodexModel
	case harness.FlagCopilot:
		return CopilotModel
	default:
		return ClaudeModel
	}
}

// Model is the model of a harness's session: the one the operator named, or the
// harness's default.
func Model(flag, chosen string) string {
	return Input{Model: chosen}.modelOr(DefaultModel(flag))
}
