package runs

import (
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Provenance lists what an attempt recorded that differs from the inputs freeze
// without voiding it (PREREG §2, §12): the Claude Code version and binary, the
// output of `shallnot --version`, the version of each toolchain the freeze
// records, and any model the usage names beyond the session model and the CLI's
// auxiliary models. A member the freeze leaves empty is not compared.
func Provenance(inputs study.Inputs, versions map[string]string, claudeSHA256, sessionModel string, usage []string) []string {
	var found []string
	differs := func(what, recorded, frozen string) {
		if frozen != "" && recorded != frozen {
			found = append(found, fmt.Sprintf("%s is %q, inputs.json holds %q", what, recorded, frozen))
		}
	}
	if version, _, _ := strings.Cut(versions[plan.ClaudeBinary], " "); inputs.ClaudeCode.Version != "" {
		differs("the Claude Code version", version, inputs.ClaudeCode.Version)
	}
	differs("the Claude Code binary's SHA-256", claudeSHA256, inputs.ClaudeCode.SHA256)
	differs("`shallnot --version`", versions[ShallnotVersionKey], inputs.Shallnot.Version)
	for _, name := range sortedKeys(inputs.Toolchains) {
		if recorded, present := versions[name]; present {
			differs("the version of "+name, recorded, inputs.Toolchains[name])
		}
	}
	for _, model := range usage {
		if model != sessionModel && !slices.Contains(inputs.CLIAuxiliaryModels, model) {
			found = append(found, fmt.Sprintf("the usage names the model %s, beyond the session model and cli_auxiliary_models %v", model, inputs.CLIAuxiliaryModels))
		}
	}
	return found
}

func sortedKeys(entries map[string]string) []string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// deviation is an entry of the deviation log for one run.
func deviation(kind study.Kind, scope, section, says, happened string) study.Deviation {
	return study.Deviation{Kind: kind, Scope: []string{scope}, Section: section, Says: says, Happened: oneLine(happened)}
}

// oneLine keeps what happened on one line of the log.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
