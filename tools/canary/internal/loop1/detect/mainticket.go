package detect

import (
	"fmt"
	"regexp"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// submitCommand matches the tickets of the three modules, which only executor
// subagents answer, in one shell command: a module's `request submit` inside one shell command: the
// binary, then its arguments up to the next command separator.
var submitCommand = regexp.MustCompile(`(?:^|[\s;&|(/])(musthold-(?:dp|alignment|completeness))(?:\s+[^;&|\n]*?)?\s+request\s+submit(?:\s|$)`)

// submitTool matches the executor tool of a module's MCP server that submits a
// ticket's answer.
var submitTool = regexp.MustCompile(`^mcp__plugin_musthold-(?:dp|alignment|completeness)_exec__submit_response$`)

// MainAgentSubmits is detector K5: the main agent, not an executor subagent, runs
// a module's `request submit`, through Bash or the module's MCP tool.
func MainAgentSubmits(calls []ToolCall) []study.Hit {
	var hits []study.Hit
	for _, call := range calls {
		if !call.Main {
			continue
		}
		var how string
		switch {
		case call.Name == ToolBash && submitCommand.MatchString(call.Command):
			how = fmt.Sprintf("Bash: %s", call.Command)
		case submitTool.MatchString(call.Name):
			how = "MCP tool " + call.Name
		default:
			continue
		}
		hits = append(hits, study.Hit{
			Detector: study.DetectorMainTicket,
			File:     StreamFile,
			Event:    call.Event,
			Evidence: "main agent submitted a ticket answer, " + how,
		})
	}
	return hits
}
