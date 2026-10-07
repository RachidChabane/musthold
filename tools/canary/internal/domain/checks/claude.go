package checks

import (
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/modules"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

// Names of the Claude Code checks.
const (
	checkClaudePluginsValid = "plugin_manifests_valid"
	checkClaudeListed       = "plugin_and_server_listed"
	checkClaudeMainDenied   = "main_session_fetch_denied"
	checkClaudeRewrite      = "rewrite_drops_model"
	checkClaudeNoBash       = "executor_without_bash"
	checkClaudeHooksFire    = "hooks_fire_under_print"
	checkClaudeCapInjected  = "hook_allow_issues_cap"

	haikuFamily      = "haiku"
	connectedStatus  = "connected"
	hookEventStop    = "Stop"
	resultSuccess    = "success"
	hookKindPreTool  = string(hostproto.HookPreTool)
	hookKindStart    = string(hostproto.HookSubagentStart)
	hookKindStop     = string(hostproto.HookSubagentStop)
	hookKindPostTool = string(hostproto.HookPostTool)
	mainAgentReason  = "the executor tools belong to the executor subagents"
)

func claudeStrategy() harness.Harness { return strategy(harness.FlagClaude) }

func claudeChecks() []Check {
	return []Check{
		versionCheck(harness.FlagClaude),
		pluginsValidCheck(),
		capWithoutHooksCheck(harness.FlagClaude),
		listedCheck(),
		mainFetchDeniedCheck(),
		rewriteDropsModelCheck(),
		hooksFireCheck(),
		capInjectedCheck(),
		executorWithoutBashCheck(),
		stopProbeCheck(claudeFinished, claudeStopEvents),
		editAppliedCheck(),
		withinCapsCheck(claudeSummary),
	}
}

func claudeStream(found evidence.Evidence) evidence.ClaudeStream {
	return evidence.DecodeClaude(found.Stream)
}

func claudeFinished(found evidence.Evidence) (bool, verdict.Pointer) {
	final, ok := claudeStream(found).Final()
	if !ok || final.Result.Subtype != resultSuccess {
		return false, verdict.Pointer{}
	}
	return true, evidence.StreamAt(final.Line, "result")
}

func claudeStopEvents(found evidence.Evidence) []verdict.Pointer {
	var pointers []verdict.Pointer
	for _, event := range claudeStream(found).HookEvents(hookEventStop) {
		pointers = append(pointers, evidence.StreamAt(event.Line, "hook_response Stop"))
	}
	return pointers
}

func claudeSummary(found evidence.Evidence) (string, bool, verdict.Pointer) {
	final, ok := claudeStream(found).Final()
	if !ok {
		return "", false, verdict.Pointer{}
	}
	summary := fmt.Sprintf("result %s after %d turns, $%.4f, %d permission denials", final.Result.Subtype, final.Result.NumTurns, final.Result.TotalCostUSD, len(final.Result.PermissionDenials))
	return summary, true, evidence.StreamAt(final.Line, "result")
}

// agentCalls returns the executor launches the main session made for the role.
func agentCalls(stream evidence.ClaudeStream, role string) []evidence.ToolUse {
	var calls []evidence.ToolUse
	for _, name := range []string{hostproto.ToolAgent, hostproto.ToolTask} {
		for _, call := range stream.ToolUses(name) {
			if call.Parent == "" && decodeAgentInput(call.Input).SubagentType == role {
				calls = append(calls, call)
			}
		}
	}
	slices.SortFunc(calls, func(a, b evidence.ToolUse) int { return a.Line - b.Line })
	return calls
}

func pluginsValidCheck() Check {
	return newCheck(checkClaudePluginsValid,
		"The harness's own validator accepts the generated plugin trees: manifest, agents, hooks and skills parse.",
		func(found evidence.Evidence) verdict.Verdict {
			results := found.NoCall.PluginValidation
			pointer := evidence.At(evidence.NoCallFile, 0, "pluginValidation")
			if len(results) == 0 {
				return verdict.Undecided("no plugin directory was validated", pointer)
			}
			var rejected []string
			for _, result := range results {
				if result.ExitCode != 0 {
					rejected = append(rejected, result.Dir)
				}
			}
			if len(rejected) > 0 {
				return verdict.Failed("the validator rejected "+strings.Join(rejected, ", "), pointer)
			}
			return verdict.Passed(fmt.Sprintf("%d plugin trees validated", len(results)), pointer)
		})
}

func listedCheck() Check {
	return newCheck(checkClaudeListed,
		"A non-interactive session loads the plugin of each module and starts its executor server: system/init lists them and reports no plugin error.",
		func(found evidence.Evidence) verdict.Verdict {
			init, ok := claudeStream(found).Init()
			if !ok {
				return verdict.Undecided("the stream holds no system/init event")
			}
			pointer := evidence.StreamAt(init.Line, "system/init")
			var missing []string
			for _, module := range modules.All() {
				if !slices.ContainsFunc(init.Init.Plugins, func(p evidence.Named) bool { return p.Name == module.Plugin }) {
					missing = append(missing, "plugin "+module.Plugin)
				}
				if !slices.ContainsFunc(init.Init.Servers, func(s evidence.Named) bool {
					return strings.Contains(s.Name, module.Plugin) && strings.Contains(s.Name, harness.ExecutorServer) && (s.Status == "" || s.Status == connectedStatus)
				}) {
					missing = append(missing, "connected server of "+module.Plugin)
				}
			}
			switch {
			case len(init.Init.PluginErrors) > 0:
				return verdict.Failed(fmt.Sprintf("system/init reports %d plugin errors: %s", len(init.Init.PluginErrors), init.Init.PluginErrors[0]), pointer)
			case len(missing) > 0:
				return verdict.Failed("system/init lacks: "+strings.Join(missing, "; "), pointer)
			default:
				return verdict.Passed(fmt.Sprintf("system/init lists %d plugins and %d servers, none failed", len(init.Init.Plugins), len(init.Init.Servers)), pointer)
			}
		})
}

func mainFetchDeniedCheck() Check {
	tool := claudeStrategy().ToolName(hostproto.ToolFetchRequest)
	return newCheck(checkClaudeMainDenied,
		"The main session cannot fetch a ticket: a hook refuses the executor tools to a caller that is no executor, so the main agent stays blind to what it must not judge.",
		func(found evidence.Evidence) verdict.Verdict {
			stream := claudeStream(found)
			var call *evidence.ToolUse
			for _, use := range stream.ToolUses(tool) {
				if use.Parent == "" {
					call = &use
					break
				}
			}
			if call == nil {
				return verdict.Undecided("the main session never called " + tool + ": nothing was probed")
			}
			callPointer := evidence.StreamAt(call.Line, "main-session call of "+tool)
			result, hasResult := stream.ResultOf(call.ID)
			if !hasResult {
				return verdict.Undecided("the call has no result in the stream", callPointer)
			}
			resultPointer := evidence.StreamAt(result.Line, "tool result")
			if !result.IsError && strings.Contains(result.Text, "nonce") {
				return verdict.Failed("the main session received a ticket", callPointer, resultPointer)
			}
			hook, hookDenied := mainSessionDecision(found, tool)
			pointers := []verdict.Pointer{callPointer, resultPointer}
			entry, journaled := mainAgentExposure(found)
			if journaled {
				pointers = append(pointers, evidence.JournalAt(entry.Line, "exposure_attempt"))
			}
			denied := hookDenied || journaled
			switch {
			case hook != nil && hook.Decision().Permission != string(hostproto.PermissionDeny):
				pointers = append(pointers, evidence.HooksAt(hook.Line, "pre-tool answer"))
				return verdict.Failed(fmt.Sprintf("the hook answered %q to the main session", hook.Decision().Permission), pointers...)
			case denied && result.IsError:
				if hook != nil {
					pointers = append(pointers, evidence.HooksAt(hook.Line, "pre-tool deny"))
				}
				return verdict.Passed("the main session's call was denied by the hook and the tool result is an error", pointers...)
			case result.IsError:
				return verdict.Undecided("the call failed but no hook denial was recorded, so the refusal is not attributed to the hook: "+truncate(result.Text), pointers...)
			default:
				return verdict.Undecided("the call returned no error and no ticket: "+truncate(result.Text), pointers...)
			}
		})
}

// mainSessionDecision finds the pre-tool hook record for a call of the tool that
// carries no agent identity, which is the main session's.
func mainSessionDecision(found evidence.Evidence, tool string) (*evidence.HookCall, bool) {
	for _, call := range found.HookCalls(primaryModule().Binary, hookKindPreTool) {
		payload := call.Payload()
		if payload.ToolName == tool && payload.AgentID == "" {
			return &call, call.Decision().Permission == string(hostproto.PermissionDeny)
		}
	}
	return nil, false
}

func mainAgentExposure(found evidence.Evidence) (evidence.JournalEntry, bool) {
	for _, entry := range found.Events(hostproto.EventExposureAttempt) {
		if entry.Event.AgentID == "" && strings.Contains(entry.Event.Detail, mainAgentReason) {
			return entry, true
		}
	}
	return evidence.JournalEntry{}, false
}

func truncate(text string) string {
	const limit = 160
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

func rewriteDropsModelCheck() Check {
	return newCheck(checkClaudeRewrite,
		"The launch hook rebuilds an executor's launch to the bare ticket: the model the main agent chose and any padding are dropped, and the executor runs on the session's model.",
		func(found evidence.Evidence) verdict.Verdict {
			stream := claudeStream(found)
			calls := agentCalls(stream, found.Session.Role)
			if len(calls) == 0 {
				return verdict.Undecided("the main session launched no " + found.Session.Role + ": " + noExecutorNote)
			}
			call := calls[0]
			requested := decodeAgentInput(call.Input)
			callPointer := evidence.StreamAt(call.Line, "Agent call as the model made it")
			bare := "ticket " + found.Session.Ticket
			if requested.Model == "" || requested.Prompt == bare {
				return verdict.Undecided("the main agent did not exercise the rewrite: its call set no model or no padding", callPointer)
			}
			hook := launchDecision(found, found.Session.Role)
			if hook == nil {
				if len(found.Hooks) == 0 {
					return verdict.Undecided("the hook log is empty: the recorder saw nothing", callPointer)
				}
				return verdict.Failed("no pre-tool hook ran on the Agent call", callPointer)
			}
			hookPointer := evidence.HooksAt(hook.Line, "pre-tool answer")
			updated := decodeAgentInput(hook.Decision().UpdatedInput)
			switch {
			case hook.Decision().Permission != string(hostproto.PermissionAllow):
				return verdict.Failed(fmt.Sprintf("the launch hook answered %q: %s", hook.Decision().Permission, hook.Decision().Reason), callPointer, hookPointer)
			case updated.Model != "" || updated.Prompt != bare || updated.RunInBackground:
				return verdict.Failed(fmt.Sprintf("the rewritten launch still carries model %q, prompt %q, background %v", updated.Model, updated.Prompt, updated.RunInBackground), callPointer, hookPointer)
			}
			pointers := []verdict.Pointer{callPointer, hookPointer}
			models := launchedModels(found, stream, call)
			for _, model := range models {
				if strings.Contains(strings.ToLower(model.name), haikuFamily) {
					return verdict.Failed(fmt.Sprintf("the executor ran on %s although the rewrite dropped the requested %q", model.name, requested.Model), append(pointers, model.pointer)...)
				}
				pointers = append(pointers, model.pointer)
			}
			return verdict.Passed(fmt.Sprintf("the model %q and the padding were dropped from the launch; executor model: %s", requested.Model, describeModels(models)), pointers...)
		})
}

// launchDecision is the pre-tool record of the executor launch for a role.
func launchDecision(found evidence.Evidence, role string) *evidence.HookCall {
	for _, call := range found.HookCalls(primaryModule().Binary, hookKindPreTool) {
		payload := call.Payload()
		if hostproto.IsAgentTool(payload.ToolName) && decodeAgentInput(payload.ToolInput).SubagentType == role {
			return &call
		}
	}
	return nil
}

type observedModel struct {
	name    string
	pointer verdict.Pointer
}

func launchedModels(found evidence.Evidence, stream evidence.ClaudeStream, call evidence.ToolUse) []observedModel {
	var models []observedModel
	for _, entry := range found.Events(hostproto.EventRunReported) {
		if entry.Event.Ticket == found.Session.Ticket && reportedModel(entry.Event.Detail) != "" {
			models = append(models, observedModel{reportedModel(entry.Event.Detail), evidence.JournalAt(entry.Line, "run_reported resolvedModel")})
		}
	}
	for _, child := range stream.Children(call.ID) {
		if child.Type == evidence.ClaudeTypeAssistant && child.Model != "" {
			models = append(models, observedModel{child.Model, evidence.StreamAt(child.Line, "subagent message model")})
			break
		}
	}
	return models
}

func describeModels(models []observedModel) string {
	if len(models) == 0 {
		return "not reported by the harness"
	}
	names := make([]string, len(models))
	for index, model := range models {
		names[index] = model.name
	}
	return strings.Join(names, ", ")
}

func hooksFireCheck() Check {
	required := []string{hookKindPreTool, hookKindStart, hookKindStop, hookKindPostTool}
	return newCheck(checkClaudeHooksFire,
		"The plugin's hooks run under non-interactive execution, inside the subagent as well: launch, start, stop and post-tool, each exiting cleanly.",
		func(found evidence.Evidence) verdict.Verdict {
			stream := claudeStream(found)
			if len(agentCalls(stream, found.Session.Role)) == 0 {
				return verdict.Undecided("the main session launched no " + found.Session.Role + ": " + noExecutorNote)
			}
			binary := primaryModule().Binary
			if !slices.ContainsFunc(found.Hooks, func(call evidence.HookCall) bool { return call.Module == binary }) {
				return verdict.Undecided("the recorder saw no command of " + binary + ", so a missing hook cannot be told from a missing recorder")
			}
			var missing []string
			var pointers []verdict.Pointer
			for _, kind := range required {
				calls := found.HookCalls(binary, kind)
				if len(calls) == 0 {
					missing = append(missing, kind)
					continue
				}
				for _, call := range calls {
					if call.ExitCode != 0 {
						return verdict.Failed(fmt.Sprintf("the %s hook exited %d", kind, call.ExitCode), evidence.HooksAt(call.Line, kind))
					}
				}
				pointers = append(pointers, evidence.HooksAt(calls[0].Line, kind))
			}
			if len(missing) > 0 {
				return verdict.Failed("hooks that never ran: "+strings.Join(missing, ", "), pointers...)
			}
			return verdict.Passed("all four hooks ran and exited 0", pointers...)
		})
}

func capInjectedCheck() Check {
	tool := claudeStrategy().ToolName(hostproto.ToolFetchRequest)
	return newCheck(checkClaudeCapInjected,
		"A hook's allow with an updated input approves a plugin MCP tool call inside a subagent under non-interactive execution, and the capability it adds is accepted by the server.",
		func(found evidence.Evidence) verdict.Verdict {
			stream := claudeStream(found)
			var fetch *evidence.HookCall
			for _, call := range found.HookCalls(primaryModule().Binary, hookKindPreTool) {
				payload := call.Payload()
				if payload.ToolName == tool && payload.AgentID != "" {
					fetch = &call
					break
				}
			}
			if fetch == nil {
				return verdict.Undecided("no executor called " + tool + ": " + noExecutorNote)
			}
			decision := fetch.Decision()
			hookPointer := evidence.HooksAt(fetch.Line, "pre-tool answer for the executor's fetch")
			if decision.Permission != string(hostproto.PermissionAllow) || capOf(decision.UpdatedInput) == "" {
				return verdict.Failed(fmt.Sprintf("the hook answered %q with no capability: %s", decision.Permission, decision.Reason), hookPointer)
			}
			if entry := found.Events(hostproto.EventUncappedSubmission); len(entry) > 0 {
				return verdict.Failed("the server journaled an uncapped submission", hookPointer, evidence.JournalAt(entry[0].Line, "uncapped_submission"))
			}
			for _, use := range stream.ToolUses(tool) {
				if use.Parent == "" {
					continue
				}
				if result, ok := stream.ResultOf(use.ID); ok && result.IsError {
					return verdict.Failed("the executor's fetch ended in an error: "+truncate(result.Text), hookPointer, evidence.StreamAt(result.Line, "tool result"))
				}
			}
			if entry, ok := hasEvent(found, hostproto.EventFetched, found.Session.Ticket); ok {
				return verdict.Passed("the hook allowed the fetch with a capability and the server journaled it as fetched", hookPointer, evidence.JournalAt(entry.Line, "fetched"))
			}
			return verdict.Undecided("the hook allowed the call with a capability, but the journal holds no fetched event for the ticket", hookPointer)
		})
}

func executorWithoutBashCheck() Check {
	allowed := claudeStrategy().ToolNames()
	return newCheck(checkClaudeNoBash,
		"The executor made no call other than fetch and submit of its ticket, and replied with the bare receipt. This shows that Bash was not used; it does not show that the tool was absent.",
		func(found evidence.Evidence) verdict.Verdict {
			started := startedAgent(found)
			if started == nil {
				return verdict.Undecided("the journal records no started executor: " + noExecutorNote)
			}
			startPointer := evidence.JournalAt(started.Line, "agent_started")
			stream := claudeStream(found)
			for _, call := range agentCalls(stream, found.Session.Role) {
				for _, child := range stream.Children(call.ID) {
					for _, block := range child.Blocks {
						if block.Type == evidence.BlockToolUse && !slices.Contains(allowed, block.Name) {
							return verdict.Failed("the executor called "+block.Name, startPointer, evidence.StreamAt(child.Line, "executor tool call"))
						}
					}
				}
			}
			for _, kind := range []hostproto.EventKind{hostproto.EventExposed, hostproto.EventAttested, hostproto.EventLeaked, hostproto.EventRestricted} {
				for _, entry := range found.Events(kind) {
					if entry.Event.AgentID != started.Event.AgentID {
						continue
					}
					pointer := evidence.JournalAt(entry.Line, string(kind))
					switch kind {
					case hostproto.EventExposed:
						return verdict.Failed("the stop hook marked the run exposed: "+entry.Event.Detail, startPointer, pointer)
					case hostproto.EventRestricted:
						return verdict.Undecided("the stop hook could not read the transcript: "+entry.Event.Detail, startPointer, pointer)
					case hostproto.EventLeaked:
						return verdict.Passed("the transcript holds only fetch and submit; the reply was not the bare receipt", startPointer, pointer)
					default:
						return verdict.Passed("the transcript holds exactly fetch and submit of the ticket and the reply is the bare receipt", startPointer, pointer)
					}
				}
			}
			return verdict.Undecided("the subagent-stop hook journaled no verdict on the run", startPointer)
		})
}

func startedAgent(found evidence.Evidence) *evidence.JournalEntry {
	for _, entry := range found.Events(hostproto.EventAgentStarted) {
		if entry.Event.Role == found.Session.Role {
			return &entry
		}
	}
	return nil
}
