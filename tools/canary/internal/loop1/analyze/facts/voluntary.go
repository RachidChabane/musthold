package facts

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/modules"
)

// The tool calls that run a proof of the session's own accord (PREREG §9.10).
const (
	toolBash         = "Bash"
	toolSkill        = "Skill"
	proveSubcommand  = " prove"
	proveAllSkill    = "prove-all"
	commandMember    = "command"
	skillMember      = "skill"
	callPrefixBash   = "Bash: "
	callPrefixSkill  = "Skill: "
	findingCodeShape = `\b[a-z]+(?:_[a-z]+)+\b`
)

// proofFindingCodes are the finding codes a proof's output may print.
var proofFindingCodes = []string{
	staleProof, "malformed_proof", "correct_code_fails", "example_failed", "surviving_valid_mutant",
	"escalated_mutant", "no_valid_mutant", "no_killed_valid_mutant", "mutation_sensitive_test",
	"misaligned_test", "completeness_question",
}

var codeShape = regexp.MustCompile(findingCodeShape)

// isFindingCode reports whether a word is a proof finding code or a pending
// reason.
func isFindingCode(word string) bool {
	return slices.Contains(proofFindingCodes, word) || slices.Contains(format.PendingReasons(), format.PendingReason(word))
}

// voluntary lists the module prove commands and prove-all skill calls of the
// session, with the finding codes their results print, sorted and distinct.
func voluntary(lines []evidence.Line) []Voluntary {
	stream := evidence.DecodeClaude(lines)
	found := []Voluntary{}
	for _, call := range stream.ToolUses("") {
		label, ok := proofCall(call)
		if !ok {
			continue
		}
		entry := Voluntary{Line: call.Line, Call: label, Findings: []string{}}
		if result, ok := stream.ResultOf(call.ID); ok {
			for _, word := range codeShape.FindAllString(result.Text, -1) {
				if isFindingCode(word) && !slices.Contains(entry.Findings, word) {
					entry.Findings = append(entry.Findings, word)
				}
			}
			slices.Sort(entry.Findings)
		}
		found = append(found, entry)
	}
	return found
}

// proofCall names a tool call that runs a module's prove or the prove-all skill.
func proofCall(call evidence.ToolUse) (string, bool) {
	var input map[string]any
	_ = json.Unmarshal(call.Input, &input)
	switch call.Name {
	case toolBash:
		command, _ := input[commandMember].(string)
		for _, module := range modules.All() {
			if strings.Contains(command, module.Binary+proveSubcommand) {
				return callPrefixBash + command, true
			}
		}
	case toolSkill:
		skill, _ := input[skillMember].(string)
		if strings.Contains(skill, proveAllSkill) {
			return callPrefixSkill + skill, true
		}
	}
	return "", false
}
