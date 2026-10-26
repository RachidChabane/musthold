// Package transcript builds the readable account of one recorded run of the
// agent loop: what the agent was asked, what it said and did, each decision of
// the gate with the message the agent received, the agent's edits, how the run
// ended, and the counts and timings the records give. Every
// text it carries comes from a recorded file, with the file and line it comes
// from; the package adds only the order and the kind of each entry. It is pure.
package transcript

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/modules"
)

// Kind is what an entry of the transcript stands for.
type Kind string

// Kinds of entries.
const (
	KindAgent     Kind = "agent"
	KindCommand   Kind = "command"
	KindEdit      Kind = "edit"
	KindSkill     Kind = "skill"
	KindExecutors Kind = "executors"
	KindRead      Kind = "read"
	KindTool      Kind = "tool"
	KindGate      Kind = "gate"
)

// Decision is what one run of the end-of-turn gate decided.
type Decision string

// Decisions of the gate's hook, from its exit code and its answer.
const (
	DecisionBlocked Decision = "blocked"
	DecisionGaveUp  Decision = "gave up"
	DecisionPassed  Decision = "passed"
	DecisionAllowed Decision = "allowed"
	DecisionFailed  Decision = "failed"
)

// Exit codes of Shallnot's Claude Code hook.
const (
	hookExitAllow  = 0
	hookExitGiveUp = 1
	hookExitBlock  = 2
)

// Names the transcript reads in the records.
const (
	shallnotModule  = "shallnot"
	hookSubcommand  = "hook"
	stopHookCommand = "claude-stop"
	toolBash        = "Bash"
	toolAgent       = "Agent"
	toolTask        = "Task"
	toolSkill       = "Skill"
	toolEdit        = "Edit"
	toolMultiEdit   = "MultiEdit"
	toolWrite       = "Write"
	unknownTime     = -1
)

var readTools = []string{"Read", "Grep", "Glob", "LS"}

// Entry is one step of the transcript.
type Entry struct {
	Kind Kind
	// AtMillis is when the step happened, in milliseconds since the session
	// started, or -1 when the records do not say.
	AtMillis int64
	// Decision is the gate's decision, for a gate entry.
	Decision Decision
	// Text is the recorded text of the step: what the agent wrote, a command, a
	// file name, the gate's message.
	Text string
	// Block is a recorded block shown as is: a command's output, a diff.
	Block string
	// Diff says whether the block is a diff.
	Diff bool
	// Count is the number of executors an executors entry stands for.
	Count int
	// Source names the record and the line the step comes from.
	Source string
}

// Stats are the counts and timings of the run, all read from the records.
type Stats struct {
	// MainMessages and SubagentMessages count the distinct assistant messages.
	MainMessages, SubagentMessages int
	// ToolCalls counts the main agent's tool calls by tool.
	ToolCalls map[string]int
	// Executors counts the subagents the main agent launched.
	Executors int
	// ProveRuns counts the main agent's commands that run a module's prove.
	ProveRuns int
	// GateRuns counts the end-of-turn gate's runs by decision.
	GateRuns map[Decision]int
	// GateMillis is the time the gate's runs took, summed.
	GateMillis int64
	// SessionMillis is the wall-clock time of the session.
	SessionMillis int64
	// Result is what the closing `result` event reports, when there is one.
	Result *Result
}

// Result is the closing event of the stream.
type Result struct {
	Subtype       string
	IsError       bool
	NumTurns      int
	DurationMS    int64
	DurationAPIMS int64
	TotalCostUSD  float64
	Line          int
}

// Transcript is the account of one run.
type Transcript struct {
	Meta    record.Meta
	Entries []Entry
	Stats   Stats
	// FinalGate is the gate's report on the project the session left, or nil.
	FinalGate *record.GateReport
	// Change is the session's change against the equipped commit.
	Change string
}

// Input is a decoded recording.
type Input struct {
	Meta     record.Meta
	Stream   []evidence.Line
	Arrivals []record.Arrival
	Hooks    []evidence.HookCall
	// FinalGate is the report written after the session, or nil.
	FinalGate *record.GateReport
	Change    string
}

// Build orders the records of a run into its transcript.
func Build(in Input) Transcript {
	arrivals := map[int]int64{}
	for _, arrival := range in.Arrivals {
		arrivals[arrival.Line] = arrival.Millis
	}
	stream := evidence.DecodeClaude(in.Stream)
	results := resultsByID(stream)
	entries, stats := streamEntries(stream, results, arrivals)
	gates := gateEntries(in, &stats)
	entries = merge(entries, gates)
	stats.Result = closingResult(in.Stream)
	stats.MainMessages, stats.SubagentMessages = messageCounts(in.Stream)
	if !in.Meta.StartedAt.IsZero() && !in.Meta.EndedAt.IsZero() {
		stats.SessionMillis = in.Meta.EndedAt.Sub(in.Meta.StartedAt).Milliseconds()
	}
	return Transcript{Meta: in.Meta, Entries: entries, Stats: stats, FinalGate: in.FinalGate, Change: in.Change}
}

func resultsByID(stream evidence.ClaudeStream) map[string]evidence.ToolResult {
	found := map[string]evidence.ToolResult{}
	for _, call := range stream.ToolUses("") {
		if result, ok := stream.ResultOf(call.ID); ok {
			found[call.ID] = result
		}
	}
	return found
}

func at(arrivals map[int]int64, line int) int64 {
	if millis, found := arrivals[line]; found {
		return millis
	}
	return unknownTime
}

func source(name string, line int) string { return fmt.Sprintf("%s:%d", name, line) }

// streamEntries reads the main agent's messages and tool calls in stream order.
// The subagents' events are counted, not shown: they are the blind executors.
func streamEntries(stream evidence.ClaudeStream, results map[string]evidence.ToolResult, arrivals map[int]int64) ([]Entry, Stats) {
	stats := Stats{ToolCalls: map[string]int{}, GateRuns: map[Decision]int{}}
	var entries []Entry
	for _, event := range stream.Events {
		if event.Type != evidence.ClaudeTypeAssistant || event.Parent != "" {
			continue
		}
		for _, block := range event.Blocks {
			entry, keep := blockEntry(block, results)
			if block.Type == evidence.BlockToolUse {
				stats.ToolCalls[block.Name]++
			}
			if !keep {
				continue
			}
			entry.AtMillis, entry.Source = at(arrivals, event.Line), source(record.StreamFile, event.Line)
			if entry.Kind == KindExecutors {
				stats.Executors++
				if last := len(entries) - 1; last >= 0 && entries[last].Kind == KindExecutors {
					entries[last].Count++
					continue
				}
			}
			if entry.Kind == KindCommand && isProve(entry.Text) {
				stats.ProveRuns++
			}
			entries = append(entries, entry)
		}
	}
	return entries, stats
}

func blockEntry(block evidence.Block, results map[string]evidence.ToolResult) (Entry, bool) {
	switch {
	case block.Type == "text":
		text := strings.TrimSpace(block.Text)
		return Entry{Kind: KindAgent, Text: text}, text != ""
	case block.Type != evidence.BlockToolUse:
		return Entry{}, false
	}
	var input map[string]any
	_ = json.Unmarshal(block.Input, &input)
	switch {
	case block.Name == toolBash:
		entry := Entry{Kind: KindCommand, Text: stringOf(input, "command")}
		if result, found := results[block.ID]; found {
			entry.Block = strings.TrimRight(result.Text, "\n")
		}
		return entry, true
	case block.Name == toolAgent || block.Name == toolTask:
		return Entry{Kind: KindExecutors, Text: stringOf(input, "subagent_type"), Count: 1}, true
	case block.Name == toolSkill:
		return Entry{Kind: KindSkill, Text: stringOf(input, "skill")}, true
	case block.Name == toolEdit:
		return Entry{Kind: KindEdit, Text: stringOf(input, "file_path"), Block: replacement(stringOf(input, "old_string"), stringOf(input, "new_string")), Diff: true}, true
	case block.Name == toolMultiEdit:
		var parts []string
		if edits, ok := input["edits"].([]any); ok {
			for _, raw := range edits {
				if edit, ok := raw.(map[string]any); ok {
					parts = append(parts, replacement(stringOf(edit, "old_string"), stringOf(edit, "new_string")))
				}
			}
		}
		return Entry{Kind: KindEdit, Text: stringOf(input, "file_path"), Block: strings.Join(parts, "\n"), Diff: true}, true
	case block.Name == toolWrite:
		return Entry{Kind: KindEdit, Text: stringOf(input, "file_path"), Block: replacement("", stringOf(input, "content")), Diff: true}, true
	case slices.Contains(readTools, block.Name):
		target := stringOf(input, "file_path")
		if target == "" {
			target = stringOf(input, "pattern")
		}
		return Entry{Kind: KindRead, Text: block.Name + " " + target}, true
	default:
		return Entry{Kind: KindTool, Text: block.Name}, true
	}
}

func stringOf(input map[string]any, key string) string {
	if value, ok := input[key].(string); ok {
		return value
	}
	return ""
}

// replacement shows an edit's old text as removed lines and its new text as
// added lines.
func replacement(old, updated string) string {
	var lines []string
	for _, line := range splitLines(old) {
		lines = append(lines, "-"+line)
	}
	for _, line := range splitLines(updated) {
		lines = append(lines, "+"+line)
	}
	return strings.Join(lines, "\n")
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// isProve reports whether a command runs a proof module's prove.
func isProve(command string) bool {
	for _, module := range modules.All() {
		if strings.Contains(command, module.Binary+" prove") {
			return true
		}
	}
	return false
}

// gateEntries reads the runs of Shallnot's end-of-turn hook from the hook log,
// each with the message the agent received.
func gateEntries(in Input, stats *Stats) []Entry {
	var entries []Entry
	for _, call := range in.Hooks {
		if call.Module != shallnotModule || len(call.Args) < 2 || call.Args[0] != hookSubcommand || call.Args[1] != stopHookCommand {
			continue
		}
		decision, message := decide(call)
		stats.GateRuns[decision]++
		stats.GateMillis += call.DurationMillis
		when := sinceStart(in.Meta.StartedAt, call.StartedAt)
		where := source(record.HooksFile, call.Line)
		entries = append(entries, Entry{Kind: KindGate, AtMillis: when, Decision: decision, Text: message, Source: where})
	}
	return entries
}

// decide reads the hook's decision from its exit code and its answer.
func decide(call evidence.HookCall) (Decision, string) {
	stderr := strings.TrimSpace(call.Stderr)
	switch call.ExitCode {
	case hookExitBlock:
		return DecisionBlocked, stderr
	case hookExitGiveUp:
		return DecisionGaveUp, stderr
	case hookExitAllow:
		var answer struct {
			SystemMessage string `json:"systemMessage"`
		}
		if json.Unmarshal([]byte(call.Stdout), &answer) == nil && answer.SystemMessage != "" {
			return DecisionPassed, answer.SystemMessage
		}
		return DecisionAllowed, ""
	default:
		return DecisionFailed, stderr
	}
}

func sinceStart(started time.Time, stamp string) int64 {
	when, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil || started.IsZero() {
		return unknownTime
	}
	return when.Sub(started).Milliseconds()
}

// merge interleaves the gate's entries with the stream's by time; an entry
// whose time is unknown keeps its place after the one before it.
func merge(stream, gates []Entry) []Entry {
	merged := append(append([]Entry{}, stream...), gates...)
	keys := make([]int64, len(merged))
	var last int64
	for index, entry := range merged {
		if index == len(stream) {
			last = 0
		}
		if entry.AtMillis >= 0 {
			last = entry.AtMillis
		}
		keys[index] = last
	}
	order := make([]int, len(merged))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(i, j int) bool { return keys[order[i]] < keys[order[j]] })
	sorted := make([]Entry, len(merged))
	for index, position := range order {
		sorted[index] = merged[position]
	}
	return sorted
}

// closingResult reads the stream's `result` event.
func closingResult(lines []evidence.Line) *Result {
	for _, line := range lines {
		var raw struct {
			Type          string  `json:"type"`
			Subtype       string  `json:"subtype"`
			IsError       bool    `json:"is_error"`
			NumTurns      int     `json:"num_turns"`
			DurationMS    int64   `json:"duration_ms"`
			DurationAPIMS int64   `json:"duration_api_ms"`
			TotalCostUSD  float64 `json:"total_cost_usd"`
		}
		if json.Unmarshal(line.Raw, &raw) != nil || raw.Type != evidence.ClaudeTypeResult {
			continue
		}
		return &Result{Subtype: raw.Subtype, IsError: raw.IsError, NumTurns: raw.NumTurns, DurationMS: raw.DurationMS,
			DurationAPIMS: raw.DurationAPIMS, TotalCostUSD: raw.TotalCostUSD, Line: line.Number}
	}
	return nil
}

// messageCounts counts the distinct assistant messages of the main agent and of
// the subagents; the stream may split one message over several events.
func messageCounts(lines []evidence.Line) (main, subagents int) {
	seen := map[string]bool{}
	for _, line := range lines {
		var raw struct {
			Type    string  `json:"type"`
			Parent  *string `json:"parent_tool_use_id"`
			Message struct {
				ID string `json:"id"`
			} `json:"message"`
		}
		if json.Unmarshal(line.Raw, &raw) != nil || raw.Type != evidence.ClaudeTypeAssistant {
			continue
		}
		key := raw.Message.ID
		if key == "" {
			key = fmt.Sprintf("line-%d", line.Number)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		if raw.Parent == nil || *raw.Parent == "" {
			main++
		} else {
			subagents++
		}
	}
	return main, subagents
}
