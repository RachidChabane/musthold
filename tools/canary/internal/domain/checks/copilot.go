package checks

import (
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/format/hostproto/pluginfiles"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/modules"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

// Names of the Copilot checks. The names of the checks that mean what a Codex check
// of the same name means are shared.
const (
	checkCopilotListed    = "plugins_and_skills_listed"
	checkCopilotTrees     = "plugin_trees_valid"
	checkCopilotAgent     = "executor_agent_starts"
	checkCopilotModel     = "session_model"
	checkCopilotHooks     = "hooks_fire"
	checkCopilotStop      = "subagent_stop_fires"
	checkCopilotNaming    = "mcp_tool_naming"
	checkCopilotAgentType = "agent_type_in_pretool"
	checkCopilotAllowlist = "agent_allowlist_honoured"

	fulfilSkillSuffix  = "-fulfil"
	copilotAgentSuffix = ".agent.md"
	copilotHooksFile   = "hooks/hooks.json"
	copilotAgentsDir   = "agents/"
	pluginSource       = "plugin"
	externalSource     = "external"
	noExecutorAgent    = "the session started no executor agent of the primary module"
)

func copilotStrategy() harness.Harness { return strategy(harness.FlagCopilot) }

func copilotChecks() []Check {
	return []Check{
		versionCheck(harness.FlagCopilot),
		sessionModelCheck(),
		capWithoutHooksCheck(harness.FlagCopilot),
		serversStartCheck(),
		pluginsAndSkillsListedCheck(),
		pluginTreesValidCheck(),
		unlessRefused(executorAgentStartsCheck()),
		unlessRefused(hooksFireCopilotCheck()),
		unlessRefused(subagentStopCheck()),
		unlessRefused(copilotToolNamingCheck()),
		unlessRefused(copilotAgentTypeCheck()),
		unlessRefused(allowlistCheck()),
		unlessRefused(failsOnlyAfterNormalEnd(editAppliedCheck())),
		withinCapsCheck(copilotSummary),
		configUntouchedCheck(),
	}
}

func copilotStream(found evidence.Evidence) evidence.CopilotStream {
	return evidence.DecodeCopilot(found.Stream)
}

func copilotSummary(found evidence.Evidence) (string, bool, verdict.Pointer) {
	closing, ended := copilotStream(found).Closing()
	if !ended {
		return "", false, verdict.Pointer{}
	}
	summary := fmt.Sprintf("the session closed (%s) after %d events", closing.Type, len(found.Stream))
	if closing.PremiumRequests != nil {
		summary += fmt.Sprintf(", using %d premium requests", *closing.PremiumRequests)
	}
	return summary, true, evidence.StreamAt(closing.Line, closing.Type)
}

// unlessRefused makes a check that reads the session inconclusive when the
// session was refused (authentication, quota or rate limit): nothing the check
// looks for could then have happened, and its absence is not the harness's doing.
func unlessRefused(check Check) Check {
	return newCheck(check.Name(), check.Proves(), func(found evidence.Evidence) verdict.Verdict {
		if refusal, refused := copilotStream(found).Refusal(); refused {
			return verdict.Undecided(fmt.Sprintf("the session was refused (%s): %s", refusal.ErrorType, truncate(refusal.Message)), evidence.StreamAt(refusal.Line, evidence.CopilotEventSessionError))
		}
		return check.Evaluate(found)
	})
}

// failsOnlyAfterNormalEnd keeps a check that reads the outcome of the agent's work
// from failing on a session that did not end by itself: a file left unchanged
// then proves nothing.
func failsOnlyAfterNormalEnd(check Check) Check {
	return newCheck(check.Name(), check.Proves(), func(found evidence.Evidence) verdict.Verdict {
		result := check.Evaluate(found)
		if _, ended := copilotStream(found).Closing(); result.Status == verdict.Fail && !ended {
			return verdict.Undecided("the session did not end normally, so the work it was asked for proves nothing: "+result.Summary, result.Evidence...)
		}
		return result
	})
}

func pluginsAndSkillsListedCheck() Check {
	return newCheck(checkCopilotListed,
		"Copilot CLI lists the portable plugin of each module among the external plugins and its fulfil skill among the plugin skills, before any session.",
		func(found evidence.Evidence) verdict.Verdict {
			pointer := evidence.At(evidence.NoCallFile, 0, "copilotListing")
			listing := found.NoCall.CopilotListing
			if listing == nil || !listing.Attempted {
				return verdict.Undecided("the plugin and skill listings did not run", pointer)
			}
			plugins, pluginsDecoded := listing.Plugins()
			skills, skillsDecoded := listing.Skills()
			if !pluginsDecoded || !skillsDecoded {
				return verdict.Undecided(fmt.Sprintf("a listing is not a JSON list (plugin list exit %d, skill list exit %d)", listing.PluginsExit, listing.SkillsExit), pointer)
			}
			var missing, unstated []string
			for _, module := range modules.All() {
				skill := module.Binary + fulfilSkillSuffix
				for _, entry := range []listedEntry{
					listedPluginEntry(plugins, module.Plugin),
					listedSkillEntry(skills, skill),
				} {
					switch entry.state() {
					case entryWrong:
						missing = append(missing, entry.describe())
					case entryUnstated:
						unstated = append(unstated, entry.describe())
					}
				}
			}
			switch {
			case len(missing) > 0:
				return verdict.Failed("not listed as an enabled external plugin or plugin skill: "+strings.Join(missing, ", "), pointer)
			case len(unstated) > 0:
				return verdict.Undecided("listed, but the listing does not say whether they are enabled and from where: "+strings.Join(unstated, ", "), pointer)
			}
			return verdict.Passed(fmt.Sprintf("%d plugins and their fulfil skills are listed and enabled", len(modules.All())), pointer)
		})
}

type entryState int

const (
	entryRight entryState = iota
	entryWrong
	entryUnstated
)

// listedEntry is the entry a listing holds for one name, with the source and the
// enabled flag it should have.
type listedEntry struct {
	kind, name, wantSource string
	present                bool
	source                 string
	enabled                *bool
}

func listedPluginEntry(plugins []evidence.ListedPlugin, name string) listedEntry {
	entry := listedEntry{kind: "plugin", name: name, wantSource: externalSource}
	if index := slices.IndexFunc(plugins, func(p evidence.ListedPlugin) bool { return p.Name == name }); index >= 0 {
		entry.present, entry.source, entry.enabled = true, plugins[index].Source, plugins[index].Enabled
	}
	return entry
}

func listedSkillEntry(skills []evidence.ListedSkill, name string) listedEntry {
	entry := listedEntry{kind: "skill", name: name, wantSource: pluginSource}
	if index := slices.IndexFunc(skills, func(s evidence.ListedSkill) bool { return s.Name == name }); index >= 0 {
		entry.present, entry.source, entry.enabled = true, skills[index].Source, skills[index].Enabled
	}
	return entry
}

// state is wrong when the entry is absent, disabled, or from another source, and
// unstated when it is present and the listing omits a member that would tell.
func (e listedEntry) state() entryState {
	switch {
	case !e.present, e.enabled != nil && !*e.enabled, e.source != "" && e.source != e.wantSource:
		return entryWrong
	case e.enabled == nil || e.source == "":
		return entryUnstated
	default:
		return entryRight
	}
}

func (e listedEntry) describe() string { return e.kind + " " + e.name }

func pluginTreesValidCheck() Check {
	return newCheck(checkCopilotTrees,
		"The portable tree of each module has the shape ADR 0009 gives it: a manifest named after the plugin, an mcp.json whose root server is the plugin's, a fulfil skill in a directory of its own name, and the Copilot agents and hooks in the Copilot namespace.",
		func(found evidence.Evidence) verdict.Verdict {
			pointer := evidence.At(evidence.NoCallFile, 0, "pluginTrees")
			if len(found.NoCall.PluginTrees) == 0 {
				return verdict.Undecided("no plugin tree was inspected", pointer)
			}
			var problems []string
			for _, module := range modules.All() {
				dir := module.PluginDir(harness.FlagPortable)
				index := slices.IndexFunc(found.NoCall.PluginTrees, func(t evidence.PluginTree) bool { return strings.HasSuffix(t.Dir, dir) })
				if index < 0 {
					return verdict.Undecided("the tree "+dir+" was not inspected", pointer)
				}
				problems = append(problems, treeProblems(module, found.NoCall.PluginTrees[index])...)
			}
			if len(problems) > 0 {
				return verdict.Failed(strings.Join(problems, "; "), pointer)
			}
			return verdict.Passed(fmt.Sprintf("%d portable trees have the expected files", len(modules.All())), pointer)
		})
}

// treeProblems lists what a module's inspected tree lacks of what its generator
// writes.
func treeProblems(module modules.Module, tree evidence.PluginTree) []string {
	var problems []string
	for _, problem := range tree.Problems {
		problems = append(problems, tree.Dir+": "+problem)
	}
	if tree.Name != "" && tree.Name != module.Plugin {
		problems = append(problems, fmt.Sprintf("%s: the manifest is named %q, the plugin is %q", tree.Dir, tree.Name, module.Plugin))
	}
	if !slices.Contains(tree.Servers, module.Plugin) {
		problems = append(problems, fmt.Sprintf("%s: mcp.json has no root server named %q", tree.Dir, module.Plugin))
	}
	skill := module.Binary + fulfilSkillSuffix
	if !slices.ContainsFunc(tree.Skills, func(s evidence.TreeSkill) bool { return s.Name == skill }) {
		problems = append(problems, fmt.Sprintf("%s: no skill named %q", tree.Dir, skill))
	}
	h := harness.Copilot(module.Identity())
	wanted := []string{pluginfiles.CopilotNamespace + "/" + copilotHooksFile}
	for _, agent := range module.Agents {
		wanted = append(wanted, pluginfiles.CopilotNamespace+"/"+copilotAgentsDir+h.AgentTypeOf(agent)+copilotAgentSuffix)
	}
	for _, path := range wanted {
		if !slices.Contains(tree.Namespace, path) {
			problems = append(problems, fmt.Sprintf("%s: no %s", tree.Dir, path))
		}
	}
	return problems
}

func executorAgentStartsCheck() Check {
	return newCheck(checkCopilotAgent,
		"The agent files of the plugin load under `copilot -p`: the harness starts a subagent of the module's executor type.",
		func(found evidence.Evidence) verdict.Verdict {
			agentType := copilotStrategy().AgentType(found.Session.Role)
			stream := copilotStream(found)
			named := func(event evidence.CopilotEvent) bool { return strings.Contains(event.AgentName, agentType) }
			for _, event := range stream.Of(evidence.CopilotEventSubagentStarted) {
				if named(event) {
					return verdict.Passed(fmt.Sprintf("subagent %q started for the executor type %s%s", event.AgentName, agentType, taskCallNote(stream, agentType)), evidence.StreamAt(event.Line, event.Type))
				}
			}
			for _, event := range stream.Of(evidence.CopilotEventSubagentFailed) {
				if named(event) {
					return verdict.Failed("the executor subagent failed before it started: "+truncate(string(event.Raw)), evidence.StreamAt(event.Line, event.Type))
				}
			}
			return verdict.Undecided(noExecutorAgent)
		})
}

func hooksFireCopilotCheck() Check {
	return newCheck(checkCopilotHooks,
		"The hooks of the plugin run under `copilot -p`: the executor tool guard runs on tool calls and exits cleanly.",
		func(found evidence.Evidence) verdict.Verdict {
			binary := primaryModule().Binary
			calls := found.HookCalls(binary, hookKindPreTool)
			if len(calls) == 0 {
				return verdict.Undecided("the recorder saw no pre-tool hook of " + binary + ": hooks did not run, or the recorder is not on the PATH of the session")
			}
			for _, call := range calls {
				if call.ExitCode != 0 {
					return verdict.Failed(fmt.Sprintf("the pre-tool hook exited %d", call.ExitCode), evidence.HooksAt(call.Line, hookKindPreTool))
				}
			}
			return verdict.Passed(fmt.Sprintf("the pre-tool hook ran %d times and exited 0", len(calls)), evidence.HooksAt(calls[0].Line, hookKindPreTool))
		})
}

func subagentStopCheck() Check {
	return newCheck(checkCopilotStop,
		"The subagentStop hook runs when an executor subagent finishes, so the attestation of its run is reached.",
		func(found evidence.Evidence) verdict.Verdict {
			binary := primaryModule().Binary
			stops := found.HookCalls(binary, hookKindStop)
			if len(stops) > 0 {
				for _, call := range stops {
					if call.ExitCode != 0 {
						return verdict.Failed(fmt.Sprintf("the subagent-stop hook exited %d", call.ExitCode), evidence.HooksAt(call.Line, hookKindStop))
					}
				}
				return verdict.Passed("the subagent-stop hook ran and exited 0", evidence.HooksAt(stops[0].Line, hookKindStop))
			}
			agentType := copilotStrategy().AgentType(found.Session.Role)
			stream := copilotStream(found)
			_, shutDownNormally := stream.Closing()
			preTool := found.HookCalls(binary, hookKindPreTool)
			for _, event := range stream.Of(evidence.CopilotEventSubagentDone) {
				if strings.Contains(event.AgentName, agentType) && len(preTool) > 0 && shutDownNormally {
					return verdict.Failed("the executor subagent finished and the pre-tool hook of the same hooks file ran, but no subagent-stop hook did",
						evidence.StreamAt(event.Line, event.Type), evidence.HooksAt(preTool[0].Line, hookKindPreTool))
				}
			}
			return verdict.Undecided("no executor subagent finished in a session whose hooks ran, so a missing subagent-stop proves nothing")
		})
}

// toolRole says which server of the primary module a tool name belongs to.
type toolRole int

const (
	toolOther toolRole = iota
	toolExecutor
	toolRoot
	toolMisspelled
)

// classifyTool tells the executor server's tool, the root portable server's tool
// (which shares the tool names and is the main agent's), a tool of the module
// spelled in a way no matcher selects, and every other tool.
func classifyTool(name string) toolRole {
	copilot := copilotStrategy()
	if _, executor := copilot.ExecutorTool(name); executor {
		return toolExecutor
	}
	plugin := primaryModule().Plugin
	for _, tool := range []string{hostproto.ToolFetchRequest, hostproto.ToolSubmitResponse} {
		switch {
		case name == plugin+"/"+tool || name == plugin+"-"+tool || name == "mcp__"+plugin+"__"+tool:
			return toolRoot
		case strings.Contains(name, plugin) && strings.Contains(name, tool):
			return toolMisspelled
		}
	}
	return toolOther
}

func copilotToolNamingCheck() Check {
	copilot := copilotStrategy()
	return newCheck(checkCopilotNaming,
		"A hook sees the executor's MCP tools under a name the module recognizes: `<plugin>-exec-<tool>` in the payload, `<plugin>-exec/<tool>` in the agent allowlist.",
		func(found evidence.Evidence) verdict.Verdict {
			var executor *evidence.HookCall
			records := found.HookCalls(primaryModule().Binary, hookKindPreTool)
			for index, call := range records {
				name := call.CopilotHook().ToolName
				switch classifyTool(name) {
				case toolMisspelled:
					return verdict.Failed(fmt.Sprintf("the hook saw %q, the module spells %q", name, copilot.ToolNames()), evidence.HooksAt(call.Line, "pre-tool payload"))
				case toolExecutor:
					if executor == nil {
						executor = &records[index]
					}
				}
			}
			if executor != nil {
				return verdict.Passed("the hook saw "+executor.CopilotHook().ToolName, evidence.HooksAt(executor.Line, "pre-tool payload"))
			}
			return namingFromStream(found, copilot)
		})
}

// namingFromStream reads the server name the stream gives an executor tool call,
// for a session whose hooks recorded none.
func namingFromStream(found evidence.Evidence, copilot harness.Harness) verdict.Verdict {
	for _, event := range copilotStream(found).Of(evidence.CopilotEventToolStart) {
		if event.MCPTool != hostproto.ToolFetchRequest && event.MCPTool != hostproto.ToolSubmitResponse {
			continue
		}
		pointer := evidence.StreamAt(event.Line, event.Type)
		switch {
		case event.MCPServer == copilot.Server():
			return verdict.Undecided("the stream shows server "+event.MCPServer+" but no hook payload recorded the tool name", pointer)
		case strings.Contains(event.MCPServer, primaryModule().Plugin) && event.MCPServer != primaryModule().Plugin:
			return verdict.Failed(fmt.Sprintf("the harness runs the executor tool on server %q, the module names it %q", event.MCPServer, copilot.Server()), pointer)
		}
	}
	return verdict.Undecided("no executor tool call was recorded")
}

// copilotAgentTypeCheck judges the identity an executor tool payload carries
// against what the harness strategy declares of it: Behaviours.IdentifiesCaller.
func copilotAgentTypeCheck() Check {
	return newCheck(checkCopilotAgentType,
		"PreToolUse carries the calling agent exactly as the harness strategy declares: the agent type of the executor where the strategy says the caller is identified, no agent identity where it says it is not.",
		func(found evidence.Evidence) verdict.Verdict {
			calls := copilotExecutorToolCalls(found)
			if len(calls) == 0 {
				return verdict.Undecided("no PreToolUse payload for an executor tool was recorded")
			}
			if copilotStrategy().Behaviours.IdentifiesCaller {
				return identityExpected(found, calls)
			}
			return identityNotDeclared(calls)
		})
}

// copilotExecutorToolCalls returns the pre-tool records of the primary module
// whose tool is an executor tool.
func copilotExecutorToolCalls(found evidence.Evidence) []evidence.HookCall {
	var calls []evidence.HookCall
	for _, call := range found.HookCalls(primaryModule().Binary, hookKindPreTool) {
		if classifyTool(call.CopilotHook().ToolName) == toolExecutor {
			calls = append(calls, call)
		}
	}
	return calls
}

// identityExpected passes when a payload names the executor agent and fails on
// the first one that does not.
func identityExpected(found evidence.Evidence, calls []evidence.HookCall) verdict.Verdict {
	want := copilotStrategy().AgentType(found.Session.Role)
	for _, call := range calls {
		payload := call.CopilotHook()
		pointer := evidence.HooksAt(call.Line, "pre-tool payload")
		if namesAgent(payload.AgentType, want) {
			return verdict.Passed(fmt.Sprintf("the payload names agentType %q for the agent %q", payload.AgentType, want), pointer)
		}
	}
	first := calls[0].CopilotHook()
	return verdict.Failed(fmt.Sprintf("the strategy declares that the caller is identified, but the payload carries agentType %q, expected %q: a hook cannot attribute the call", first.AgentType, want),
		evidence.HooksAt(calls[0].Line, "pre-tool payload"))
}

// identityNotDeclared passes when no payload carries an agent identity, as the
// strategy declares, and fails when one does: the strategy then needs revisiting.
func identityNotDeclared(calls []evidence.HookCall) verdict.Verdict {
	for _, call := range calls {
		payload := call.CopilotHook()
		if payload.AgentID != "" || payload.AgentType != "" {
			return verdict.Failed(fmt.Sprintf("the payload carries agentId %q and agentType %q although the strategy declares that the harness does not identify the caller: revisit the strategy", payload.AgentID, payload.AgentType),
				evidence.HooksAt(call.Line, "pre-tool payload"))
		}
	}
	return verdict.Passed(fmt.Sprintf("the %d executor tool payloads carry no agent identity, as the strategy declares for this harness (IdentifiesCaller is false)", len(calls)),
		evidence.HooksAt(calls[0].Line, "pre-tool payload"))
}

// pluginPrefixSeparators are the characters a client may put between a plugin's
// name and the name of one of its agents.
const pluginPrefixSeparators = ":/.-_@"

// namesAgent reports whether a name is the agent's plain name or the name with a
// plugin prefix in front of it.
func namesAgent(got, want string) bool {
	if got == want {
		return true
	}
	prefix, found := strings.CutSuffix(got, want)
	return found && prefix != "" && strings.ContainsRune(pluginPrefixSeparators, rune(prefix[len(prefix)-1]))
}

// taskCallNote names the agent_type the main agent passed to the task tool for
// the executor, which shows whether the client spells a plugin's agent with a prefix.
func taskCallNote(stream evidence.CopilotStream, agentType string) string {
	for _, event := range stream.Of(evidence.CopilotEventToolStart) {
		if got, isTask := event.TaskAgentType(); isTask && namesAgent(got, agentType) {
			return fmt.Sprintf("; the task tool was called with agent_type %q", got)
		}
	}
	return ""
}

func sessionModelCheck() Check {
	return newCheck(checkCopilotModel,
		"The session ran on the model the canary asked for: the model that model.call_start reports is the one the session was launched with.",
		func(found evidence.Evidence) verdict.Verdict {
			event, reported := copilotStream(found).SelectedModel()
			if !reported {
				return verdict.Undecided("the stream holds no model.call_start that names the model, so the model the session ran on is unknown")
			}
			pointer := evidence.StreamAt(event.Line, event.Type)
			if event.Model != found.Session.Model {
				return verdict.Undecided(fmt.Sprintf("the session was launched with model %q and selected %q", found.Session.Model, event.Model), pointer)
			}
			return verdict.Passed("the session ran on model "+event.Model, pointer)
		})
}
