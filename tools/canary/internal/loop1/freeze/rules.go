package freeze

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Timeline is every instant PREREG §13.5 orders.
type Timeline struct {
	// Prereg, Amendment and Inputs are the generation times of the three tokens.
	Prereg, Amendment, Inputs time.Time
	Calls                     []ConstructionCall
	Canary                    []study.CanaryRecord
	Runs                      []RunStart
}

// OrderFailures lists the ways the generation times break the order of §13.5:
// the pre-registration and its amendment precede every construction call and every
// canary session; the inputs follow them and precede the start of every run.
func OrderFailures(t Timeline) []string {
	var failures []string
	add := func(format string, args ...any) { failures = append(failures, fmt.Sprintf(format, args...)) }
	if len(t.Calls) == 0 {
		add("inputs.json records no construction call")
	}
	if len(t.Canary) == 0 {
		add("inputs.json records no canary session")
	}
	for _, token := range []struct {
		tag string
		at  time.Time
	}{{PreregTag, t.Prereg}, {AmendmentTag, t.Amendment}, {InputsTag, t.Inputs}} {
		if token.at.IsZero() {
			add("the generation time of %s is unknown", token.tag)
		}
	}
	if len(failures) > 0 {
		return failures
	}
	for index, call := range t.Calls {
		if call.Start.IsZero() {
			add("construction call %d has no start", index+1)
			continue
		}
		requireBefore(add, PreregTag, t.Prereg, fmt.Sprintf("construction call %d start", index+1), call.Start)
		requireBefore(add, AmendmentTag, t.Amendment, fmt.Sprintf("construction call %d start", index+1), call.Start)
		requireBefore(add, fmt.Sprintf("construction call %d end", index+1), later(call.Start, call.End), InputsTag, t.Inputs)
	}
	for _, record := range t.Canary {
		requireBefore(add, PreregTag, t.Prereg, "canary session "+record.Name, record.Start)
		requireBefore(add, AmendmentTag, t.Amendment, "canary session "+record.Name, record.Start)
		requireBefore(add, "canary session "+record.Name, record.Start, InputsTag, t.Inputs)
	}
	for _, run := range t.Runs {
		requireBefore(add, InputsTag, t.Inputs, "start of "+run.Name, run.Start)
	}
	return failures
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func requireBefore(add func(string, ...any), earlier string, earlierAt time.Time, later string, laterAt time.Time) {
	if laterAt.IsZero() || !earlierAt.Before(laterAt) {
		add("%s (%s) does not precede %s (%s)", earlier, formatTime(earlierAt), later, formatTime(laterAt))
	}
}

func formatTime(at time.Time) string {
	if at.IsZero() {
		return "no time"
	}
	return at.UTC().Format(time.RFC3339Nano)
}

// InstructionFailures lists, for one task, what differs between the arms'
// instructions other than the arm-specific file (PREREG §5): the prompt, the
// session settings, the plugin trees and every other file of the equipped commit.
func InstructionFailures(task study.TaskInputs) []string {
	arms := slices.Sorted(maps.Keys(task.Instructions))
	if len(arms) < 2 {
		return []string{fmt.Sprintf("task %s records the instructions of %d arms", task.Name, len(arms))}
	}
	first := task.Instructions[arms[0]]
	var failures []string
	for _, arm := range arms[1:] {
		other := task.Instructions[arm]
		for _, difference := range armDifferences(first, other) {
			failures = append(failures, fmt.Sprintf("task %s: %s and %s differ in %s", task.Name, arms[0], arm, difference))
		}
	}
	return failures
}

func armDifferences(a, b study.ArmInstructions) []string {
	var differences []string
	if a.Prompt != b.Prompt {
		differences = append(differences, "the prompt")
	}
	if a.SessionSettings != b.SessionSettings {
		differences = append(differences, "session-settings.json")
	}
	if !maps.Equal(a.Plugins, b.Plugins) {
		differences = append(differences, "the plugin trees")
	}
	for _, file := range differingKeys(withoutArmFile(a.Files), withoutArmFile(b.Files)) {
		differences = append(differences, "the file "+file)
	}
	return differences
}

func withoutArmFile(files map[string]string) map[string]string {
	kept := maps.Clone(files)
	delete(kept, ArmSpecificFile)
	return kept
}

func differingKeys(a, b map[string]string) []string {
	var keys []string
	for key, digest := range a {
		if other, ok := b[key]; !ok || other != digest {
			keys = append(keys, key)
		}
	}
	for key := range b {
		if _, ok := a[key]; !ok {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

// PolicyFailures lists the members of got that differ from want, and the members
// of want that got lacks.
func PolicyFailures(label string, got, want map[string]map[string]string) []string {
	var failures []string
	for _, module := range slices.Sorted(maps.Keys(want)) {
		for _, member := range slices.Sorted(maps.Keys(want[module])) {
			value, ok := got[module][member]
			if !ok || value != want[module][member] {
				failures = append(failures, fmt.Sprintf("%s: %s.%s is %q, the protocol says %q", label, module, member, value, want[module][member]))
			}
		}
	}
	return failures
}

// PinFiles are the contents of the files that hold the pins, at the inputs tag.
type PinFiles struct {
	Shallnot, ClaudeCode, RequirementsLock string
}

var lockedPytest = regexp.MustCompile(`(?m)^pytest==([0-9][0-9.]*)`)

// PinFailures lists the pins and toolchain versions of the inputs that differ from
// PREREG §2, as the amendment sets the Shallnot pin, and from the pytest version
// of the image's lock file.
func PinFailures(inputs study.Inputs, files PinFiles) []string {
	shallnotPinFile, claudePinFile := files.Shallnot, files.ClaudeCode
	var failures []string
	add := func(format string, args ...any) { failures = append(failures, fmt.Sprintf(format, args...)) }
	if inputs.Shallnot.Pin != ExpectedShallnotPin || strings.TrimSpace(shallnotPinFile) != ExpectedShallnotPin {
		add("the Shallnot pin is %q in inputs.json and %q in %s, the protocol says %s", inputs.Shallnot.Pin, strings.TrimSpace(shallnotPinFile), ShallnotPinFile, ExpectedShallnotPin)
	}
	if inputs.Shallnot.Version == "" || inputs.Shallnot.Release == "" {
		add("inputs.json records no Shallnot version or release")
	}
	if inputs.ClaudeCode.Version != ExpectedClaudeVersion || strings.TrimSpace(claudePinFile) != ExpectedClaudeVersion {
		add("Claude Code is %q in inputs.json and %q in %s, the protocol says %s", inputs.ClaudeCode.Version, strings.TrimSpace(claudePinFile), ClaudeCodePinFile, ExpectedClaudeVersion)
	}
	if !digestPattern.MatchString(inputs.ClaudeCode.SHA256) {
		add("inputs.json records no SHA-256 of the Claude Code binary")
	}
	for _, key := range slices.Sorted(maps.Keys(ExpectedToolchains)) {
		if got := inputs.Toolchains[key]; got != ExpectedToolchains[key] {
			add("toolchain %s is %q, the protocol says %q", key, got, ExpectedToolchains[key])
		}
	}
	if match := lockedPytest.FindStringSubmatch(files.RequirementsLock); match == nil || inputs.Toolchains[ToolchainPytest] != match[1] {
		add("toolchain %s is %q, the image's lock file does not pin the same version", ToolchainPytest, inputs.Toolchains[ToolchainPytest])
	}
	return failures
}
