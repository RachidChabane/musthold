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

// bookkeepingTools are the tools the CLI itself calls to narrate its work. They
// are not a use of the shell or of the project, so they do not count against an
// agent's tool allowlist.
var bookkeepingTools = []string{"report_intent"}

func copilotStrategy() harness.Harness { return strategy(harness.FlagCopilot) }

func copilotChecks() []Check {
	return []Check{
		versionCheck(harness.FlagCopilot),
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
	return fmt.Sprintf("the session closed (%s) after %d events", closing.Type, len(found.Stream)), true, evidence.StreamAt(closing.Line, closing.Type)
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
					return verdict.Passed(fmt.Sprintf("subagent %q started for the executor type %s", event.AgentName, agentType), evidence.StreamAt(event.Line, event.Type))
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
		case name == plugin+"/"+tool || name == "mcp__"+plugin+"__"+tool:
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
		"A hook sees the executor's MCP tools under the name the module's matchers and allowlists spell, `<plugin>-exec/<tool>`.",
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

func copilotAgentTypeCheck() Check {
	return newCheck(checkCopilotAgentType,
		"PreToolUse names the calling agent: the payload of an executor tool call carries the agent type, so a hook can tell the executor from the main agent. ADR 0005 leaves this unverified.",
		func(found evidence.Evidence) verdict.Verdict {
			want := copilotStrategy().AgentType(found.Session.Role)
			var empty *evidence.HookCall
			records := found.HookCalls(primaryModule().Binary, hookKindPreTool)
			for index, call := range records {
				payload := call.CopilotHook()
				if classifyTool(payload.ToolName) != toolExecutor {
					continue
				}
				pointer := evidence.HooksAt(call.Line, "pre-tool payload")
				switch payload.AgentType {
				case want:
					return verdict.Passed(fmt.Sprintf("the payload names agentType %q", want), pointer)
				case "":
					empty = &records[index]
				default:
					return verdict.Failed(fmt.Sprintf("the payload carries agentType %q, expected %q: a hook cannot attribute the call", payload.AgentType, want), pointer)
				}
			}
			if empty != nil {
				return verdict.Undecided("the payload of an executor tool call carries no agentType", evidence.HooksAt(empty.Line, "pre-tool payload"))
			}
			return verdict.Undecided("no PreToolUse payload for an executor tool was recorded")
		})
}

// isExecutorTool reports whether a tool.execution_start is a tool of the executor
// server of the primary module.
func isExecutorTool(event evidence.CopilotEvent) bool {
	if event.MCPServer == copilotStrategy().Server() {
		return true
	}
	_, executor := copilotStrategy().ExecutorTool(event.ToolName)
	return executor
}

// allowlistCheck reads what the executor subagent used besides its two tools.
// Events a subagent originated carry the envelope's agentId; the session starts
// one subagent.
func allowlistCheck() Check {
	return newCheck(checkCopilotAllowlist,
		"The executor agent's MCP-only tool allowlist is honoured: a shell command it was asked to run did not run and left no file. A session in which the executor never tried leaves the question open; ADR 0005 marks the allowlist unverified.",
		func(found evidence.Evidence) verdict.Verdict {
			probe := found.SandboxProbe
			probePointer := evidence.At(evidence.SandboxProbeFile, 0, probe.File)
			stream := copilotStream(found)
			var ran, blocked, unknown []evidence.CopilotEvent
			for _, event := range stream.Of(evidence.CopilotEventToolStart) {
				if event.AgentID == "" || isExecutorTool(event) || slices.Contains(bookkeepingTools, event.ToolName) {
					continue
				}
				switch completion, closed := stream.CompletionOf(event.ToolCallID); {
				case closed && completion.Success != nil && *completion.Success:
					ran = append(ran, event)
				case closed && completion.Success != nil:
					blocked = append(blocked, event)
				default:
					unknown = append(unknown, event)
				}
			}
			mainAgent := mainAgentAttempts(stream, probe.File)
			switch {
			case len(ran) > 0:
				return verdict.Failed("the executor ran "+toolNames(ran)+" outside its allowlist", evidence.StreamAt(ran[0].Line, ran[0].Type))
			case probe.File == "":
				return verdict.Undecided("the allowlist probe was not recorded")
			case probe.Exists && len(mainAgent) > 0:
				return verdict.Undecided(probe.File+" exists but the main agent ran the command itself, so the write is not the executor's", probePointer, evidence.StreamAt(mainAgent[0].Line, mainAgent[0].Type))
			case probe.Exists && len(blocked)+len(unknown) > 0:
				return verdict.Failed("the executor tried "+toolNames(append(blocked, unknown...))+", the main agent did not, and "+probe.File+" exists: its allowlist did not hold", probePointer)
			case probe.Exists:
				return verdict.Undecided(probe.File+" exists but the stream attributes no attempt to the executor", probePointer)
			case len(blocked) > 0:
				return verdict.Passed(fmt.Sprintf("the executor tried %s, which failed, and %s does not exist", toolNames(blocked), probe.File), probePointer, evidence.StreamAt(blocked[0].Line, blocked[0].Type))
			case len(unknown) > 0:
				return verdict.Undecided("the executor tried "+toolNames(unknown)+" but the stream does not say how it ended", evidence.StreamAt(unknown[0].Line, unknown[0].Type))
			default:
				return verdict.Undecided("the executor tried nothing outside its two tools, so the allowlist was not exercised", probePointer)
			}
		})
}

// mainAgentAttempts returns the tool calls of the main agent, other than the one
// that delegates to a subagent, whose JSON names the probe file: a main agent that
// ran the command itself makes the file no evidence about the executor.
func mainAgentAttempts(stream evidence.CopilotStream, file string) []evidence.CopilotEvent {
	if file == "" {
		return nil
	}
	delegations := map[string]bool{}
	for _, event := range stream.Of(evidence.CopilotEventSubagentStarted) {
		delegations[event.ToolCallID] = true
	}
	var attempts []evidence.CopilotEvent
	for _, event := range stream.Of(evidence.CopilotEventToolStart) {
		if event.AgentID == "" && !delegations[event.ToolCallID] && event.Mentions(file) {
			attempts = append(attempts, event)
		}
	}
	return attempts
}

func toolNames(events []evidence.CopilotEvent) string {
	names := make([]string, len(events))
	for index, event := range events {
		names[index] = event.ToolName
	}
	return strings.Join(names, ", ")
}
