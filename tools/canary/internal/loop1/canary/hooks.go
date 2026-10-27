package canary

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/modules"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
)

// PluginHooksFile is where a Claude Code plugin tree declares its hooks, relative
// to the tree.
const PluginHooksFile = "hooks/hooks.json"

// hookSubcommand is the word that follows a module binary in a hook command.
const hookSubcommand = "hook"

// DeclaredHook is a hook a plugin tree declares as a command of a module binary:
// `<binary> hook <kind> …`. The recording shims log exactly these commands.
type DeclaredHook struct {
	Plugin string
	Event  string
	Binary string
	Kind   string
}

// DeclaredHooks reads a plugin tree's hooks.json. A command that is not a module
// binary's hook command, such as a script of the plugin, is not recorded by a
// shim and is left out.
func DeclaredHooks(plugin string, declaration []byte) ([]DeclaredHook, error) {
	var parsed struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(declaration, &parsed); err != nil {
		return nil, fmt.Errorf("%s: %w", PluginHooksFile, err)
	}
	var declared []DeclaredHook
	for event, groups := range parsed.Hooks {
		for _, group := range groups {
			for _, hook := range group.Hooks {
				words := strings.Fields(hook.Command)
				if len(words) >= 3 && words[1] == hookSubcommand {
					declared = append(declared, DeclaredHook{Plugin: plugin, Event: event, Binary: words[0], Kind: words[2]})
				}
			}
		}
	}
	slices.SortFunc(declared, func(a, b DeclaredHook) int { return strings.Compare(a.Event+a.Binary+a.Kind, b.Event+b.Binary+b.Kind) })
	return declared, nil
}

// HookProblems names every declared hook that no call of the hook log shows.
func HookProblems(declared []DeclaredHook, calls []evidence.HookCall) []string {
	var problems []string
	for _, hook := range declared {
		called := slices.ContainsFunc(calls, func(call evidence.HookCall) bool {
			return call.IsModuleHook() && call.Module == hook.Binary && call.Kind() == hook.Kind
		})
		if !called {
			problems = append(problems, fmt.Sprintf("the %s hook of the plugin %s (%s %s %s) was not called in the session", hook.Event, hook.Plugin, hook.Binary, hookSubcommand, hook.Kind))
		}
	}
	return problems
}

// hookProblems reads the hooks the treatment arm's plugin trees declare in the
// product and checks them against the session's hook log. Each module plugin
// must declare hooks; a plugin that is not a module is checked when it declares
// module commands.
func (c Canary) hookProblems(inv Invocation, result runs.Result) ([]string, error) {
	var problems []string
	var hooks []DeclaredHook
	layout := plan.Layout{Product: inv.Plan.Options.Product}
	for _, plugin := range plan.Plugins(layout, config.ArmTreatment) {
		declaration, err := c.Runner.App.Files.Read(filepath.Join(plugin.Dir, filepath.FromSlash(PluginHooksFile)))
		if err != nil {
			return nil, err
		}
		if declaration == nil {
			if isModulePlugin(plugin.Name) {
				problems = append(problems, "the plugin tree of "+plugin.Name+" declares no hooks")
			}
			continue
		}
		found, err := DeclaredHooks(plugin.Name, declaration)
		if err != nil {
			problems = append(problems, plugin.Name+": "+err.Error())
			continue
		}
		hooks = append(hooks, found...)
	}
	log, err := c.Runner.App.Files.Read(result.Layout.Record(record.HooksFile))
	if err != nil {
		return nil, err
	}
	calls, err := evidence.DecodeHooks(log)
	if err != nil {
		return append(problems, err.Error()), nil
	}
	return append(problems, HookProblems(hooks, calls)...), nil
}

func isModulePlugin(name string) bool {
	return slices.ContainsFunc(modules.All(), func(module modules.Module) bool { return module.Plugin == name })
}
