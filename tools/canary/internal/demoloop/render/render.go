// Package render writes a transcript as Markdown and as a self-contained HTML
// fragment. The headings and labels are the renderer's; every other text is the
// transcript's, which comes from the records, and the HTML escapes all of it.
// The same transcript always gives the same bytes.
package render

import (
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/transcript"
)

// Labels of the entries, by kind.
var kindLabels = map[transcript.Kind]string{
	transcript.KindAgent:          "Agent",
	transcript.KindCommand:        "Command",
	transcript.KindEdit:           "Edit",
	transcript.KindSkill:          "Skill",
	transcript.KindExecutors:      "Executors",
	transcript.KindRead:           "Read",
	transcript.KindTool:           "Tool",
	transcript.KindGate:           "Gate",
	transcript.KindCounterexample: "Counterexample",
}

// Words of the rendered text.
const (
	unknownClock     = "--:--"
	noOutput         = "(no output)"
	noCounterexample = "(no diff was recorded for this mutant)"
	millisPerSecond  = 1000
	secondsPerMinute = 60
	minutesPerHour   = 60
	fenceRune        = "`"
	minimumFence     = 3
	diffAdded        = "+"
	diffRemoved      = "-"
	diffHunk         = "@@"
)

// title is the heading of a transcript.
func title(t transcript.Transcript) string {
	return fmt.Sprintf("Agent loop: %s, arm %s, repetition %d", t.Meta.Task, t.Meta.Arm, t.Meta.Repetition)
}

// clock renders a time since the session started as m:ss, or h:mm:ss past an hour.
func clock(millis int64) string {
	if millis < 0 {
		return unknownClock
	}
	seconds := millis / millisPerSecond
	hours, minutes := seconds/(secondsPerMinute*minutesPerHour), (seconds/secondsPerMinute)%minutesPerHour
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds%secondsPerMinute)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds%secondsPerMinute)
}

// duration renders milliseconds as seconds with one decimal.
func duration(millis int64) string {
	return strconv.FormatFloat(float64(millis)/millisPerSecond, 'f', 1, 64) + " s"
}

// heading is the label line of an entry.
func heading(entry transcript.Entry) string {
	label := kindLabels[entry.Kind]
	switch entry.Kind {
	case transcript.KindGate:
		return label + ": " + string(entry.Decision)
	case transcript.KindCounterexample:
		return label + ": mutant " + entry.Text
	case transcript.KindExecutors:
		return fmt.Sprintf("%s: %d launched", label, entry.Count)
	}
	return label
}

// body is the text an entry shows above its block.
func body(entry transcript.Entry) string {
	switch entry.Kind {
	case transcript.KindCounterexample:
		return ""
	case transcript.KindCommand:
		return "$ " + entry.Text
	}
	return entry.Text
}

// block is the block an entry shows.
func block(entry transcript.Entry) string {
	switch {
	case entry.Kind == transcript.KindCounterexample && entry.Block == "":
		return noCounterexample
	case entry.Kind == transcript.KindCommand && entry.Block == "":
		return noOutput
	}
	return entry.Block
}

// facts are the lines that describe the run.
func facts(t transcript.Transcript) []string {
	lines := []string{"Model: " + t.Meta.Model}
	names := make([]string, 0, len(t.Meta.Versions))
	for name := range t.Meta.Versions {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		lines = append(lines, fmt.Sprintf("%s: %s", name, t.Meta.Versions[name]))
	}
	if !t.Meta.StartedAt.IsZero() {
		lines = append(lines, "Started: "+t.Meta.StartedAt.UTC().Format("2006-01-02 15:04:05 MST"))
	}
	lines = append(lines, fmt.Sprintf("Caps: %d assistant messages, %d s, %s USD", t.Meta.EventCap, t.Meta.TimeoutSeconds, t.Meta.BudgetUSD))
	return lines
}

// outcome are the lines that say how the run ended.
func outcome(t transcript.Transcript) []string {
	var lines []string
	if t.Meta.Killed != "" {
		lines = append(lines, fmt.Sprintf("Session: ended by the runner (%s), exit code %d", t.Meta.Killed, t.Meta.ExitCode))
	} else {
		lines = append(lines, fmt.Sprintf("Session: ended by itself, exit code %d", t.Meta.ExitCode))
	}
	if result := t.Stats.Result; result != nil {
		lines = append(lines, fmt.Sprintf("Result event: %s, error %t, %d turns (%s:%d)", result.Subtype, result.IsError, result.NumTurns, record.StreamFile, result.Line))
	} else {
		lines = append(lines, "Result event: none in "+record.StreamFile)
	}
	if t.FinalGate == nil {
		return append(lines, "Gate after the session: no report in "+record.FinalGateFile)
	}
	lines = append(lines, fmt.Sprintf("Gate after the session: %s, exit code %d (%s)", t.FinalGate.Verdict, t.FinalGate.ExitCode, record.FinalGateFile))
	for _, finding := range t.FinalGate.Findings {
		if finding.Blocking {
			lines = append(lines, fmt.Sprintf("Blocking: %s: %s", finding.Category, finding.Message))
		}
	}
	return lines
}

// count is one row of the counts table.
type count struct{ name, value string }

func counts(t transcript.Transcript) []count {
	stats := t.Stats
	rows := []count{
		{"Assistant messages, main agent", strconv.Itoa(stats.MainMessages)},
		{"Assistant messages, subagents", strconv.Itoa(stats.SubagentMessages)},
		{"Executors launched", strconv.Itoa(stats.Executors)},
		{"Prove runs", strconv.Itoa(stats.ProveRuns)},
	}
	for _, decision := range []transcript.Decision{transcript.DecisionBlocked, transcript.DecisionGaveUp, transcript.DecisionPassed, transcript.DecisionAllowed, transcript.DecisionFailed} {
		if runs := stats.GateRuns[decision]; runs > 0 {
			rows = append(rows, count{"Gate runs, " + string(decision), strconv.Itoa(runs)})
		}
	}
	rows = append(rows, count{"Time in the gate", duration(stats.GateMillis)})
	tools := make([]string, 0, len(stats.ToolCalls))
	for tool := range stats.ToolCalls {
		tools = append(tools, tool)
	}
	slices.Sort(tools)
	for _, tool := range tools {
		rows = append(rows, count{"Tool calls, " + tool, strconv.Itoa(stats.ToolCalls[tool])})
	}
	rows = append(rows, count{"Session wall-clock time", duration(stats.SessionMillis)})
	if result := stats.Result; result != nil {
		rows = append(rows,
			count{"Duration reported by the CLI", duration(result.DurationMS)},
			count{"API time reported by the CLI", duration(result.DurationAPIMS)},
			count{"Cost reported by the CLI", strconv.FormatFloat(result.TotalCostUSD, 'f', 4, 64) + " USD"},
			count{"Turns reported by the CLI", strconv.Itoa(result.NumTurns)},
		)
	}
	return rows
}

// fence is a code fence longer than any run of backticks in the text.
func fence(text string) string {
	longest, run := 0, 0
	for _, r := range text {
		if string(r) == fenceRune {
			run++
			longest = max(longest, run)
			continue
		}
		run = 0
	}
	return strings.Repeat(fenceRune, max(minimumFence, longest+1))
}

// Markdown renders the transcript as Markdown.
func Markdown(t transcript.Transcript) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n", title(t))
	for _, line := range facts(t) {
		fmt.Fprintf(&out, "- %s\n", line)
	}
	out.WriteString("\n## Task\n\n")
	writeFenced(&out, "text", t.Meta.Prompt)
	out.WriteString("## Session\n\n")
	for _, entry := range t.Entries {
		fmt.Fprintf(&out, "### %s · %s\n\n", clock(entry.AtMillis), heading(entry))
		fmt.Fprintf(&out, "Source: `%s`\n\n", entry.Source)
		if text := body(entry); text != "" {
			language := "text"
			if entry.Kind == transcript.KindCommand {
				language = "console"
			}
			writeFenced(&out, language, text)
		}
		if shown := block(entry); shown != "" {
			language := "text"
			if entry.Diff {
				language = "diff"
			}
			writeFenced(&out, language, shown)
		}
	}
	out.WriteString("## Outcome\n\n")
	for _, line := range outcome(t) {
		fmt.Fprintf(&out, "- %s\n", line)
	}
	if t.Change != "" {
		fmt.Fprintf(&out, "\n## Change the session made\n\nSource: `%s`\n\n", record.ChangeFile)
		writeFenced(&out, "diff", strings.TrimRight(t.Change, "\n"))
	}
	out.WriteString("\n## Counts and timings\n\n| Measure | Value |\n|---|---|\n")
	for _, row := range counts(t) {
		fmt.Fprintf(&out, "| %s | %s |\n", row.name, row.value)
	}
	return out.String()
}

func writeFenced(out *strings.Builder, language, text string) {
	marker := fence(text)
	fmt.Fprintf(out, "%s%s\n%s\n%s\n\n", marker, language, text, marker)
}

// style is the stylesheet of the fragment, scoped to its section.
const style = `.spec-proof-loop{font-family:system-ui,sans-serif;line-height:1.45;max-width:72rem}
.spec-proof-loop pre{white-space:pre-wrap;overflow-wrap:anywhere;background:rgba(127,127,127,.08);padding:.6rem .8rem;border-radius:6px;font-size:.85rem}
.spec-proof-loop .entry{border-left:3px solid rgba(127,127,127,.35);padding:.1rem 0 .1rem .8rem;margin:1rem 0}
.spec-proof-loop .entry-gate{border-left-color:#b3541e}
.spec-proof-loop .entry-gate.passed{border-left-color:#2f7d32}
.spec-proof-loop .entry-counterexample{border-left-color:#8e2430}
.spec-proof-loop .meta{opacity:.7;font-size:.8rem;margin-left:.5rem}
.spec-proof-loop .add{color:#2f7d32}
.spec-proof-loop .del{color:#b3261e}
.spec-proof-loop .hunk{opacity:.7}
.spec-proof-loop table{border-collapse:collapse}
.spec-proof-loop td,.spec-proof-loop th{border-bottom:1px solid rgba(127,127,127,.25);padding:.2rem .8rem .2rem 0;text-align:left}`

// HTML renders the transcript as a self-contained HTML fragment.
func HTML(t transcript.Transcript) string {
	var out strings.Builder
	out.WriteString(`<section class="spec-proof-loop">` + "\n<style>\n" + style + "\n</style>\n")
	fmt.Fprintf(&out, "<h2>%s</h2>\n<ul>\n", escape(title(t)))
	for _, line := range facts(t) {
		fmt.Fprintf(&out, "<li>%s</li>\n", escape(line))
	}
	out.WriteString("</ul>\n<h3>Task</h3>\n")
	fmt.Fprintf(&out, "<pre>%s</pre>\n<h3>Session</h3>\n", escape(t.Meta.Prompt))
	for _, entry := range t.Entries {
		classes := "entry entry-" + string(entry.Kind)
		if entry.Kind == transcript.KindGate {
			classes += " " + strings.ReplaceAll(string(entry.Decision), " ", "-")
		}
		fmt.Fprintf(&out, `<article class="%s">`+"\n", escape(classes))
		fmt.Fprintf(&out, "<h4><time>%s</time> %s<span class=\"meta\">%s</span></h4>\n", escape(clock(entry.AtMillis)), escape(heading(entry)), escape(entry.Source))
		if text := body(entry); text != "" {
			fmt.Fprintf(&out, "<pre>%s</pre>\n", escape(text))
		}
		if shown := block(entry); shown != "" {
			fmt.Fprintf(&out, "<pre>%s</pre>\n", blockHTML(shown, entry.Diff))
		}
		out.WriteString("</article>\n")
	}
	out.WriteString("<h3>Outcome</h3>\n<ul>\n")
	for _, line := range outcome(t) {
		fmt.Fprintf(&out, "<li>%s</li>\n", escape(line))
	}
	out.WriteString("</ul>\n")
	if t.Change != "" {
		fmt.Fprintf(&out, "<h3>Change the session made</h3>\n<p class=\"meta\">%s</p>\n<pre>%s</pre>\n", escape(record.ChangeFile), blockHTML(strings.TrimRight(t.Change, "\n"), true))
	}
	out.WriteString("<h3>Counts and timings</h3>\n<table>\n<tr><th>Measure</th><th>Value</th></tr>\n")
	for _, row := range counts(t) {
		fmt.Fprintf(&out, "<tr><td>%s</td><td>%s</td></tr>\n", escape(row.name), escape(row.value))
	}
	out.WriteString("</table>\n</section>\n")
	return out.String()
}

func escape(text string) string { return html.EscapeString(text) }

// blockHTML escapes a block, marking the added, removed and hunk lines of a diff.
func blockHTML(text string, diff bool) string {
	if !diff {
		return escape(text)
	}
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		class := ""
		switch {
		case strings.HasPrefix(line, diffHunk):
			class = "hunk"
		case strings.HasPrefix(line, diffAdded) && !strings.HasPrefix(line, "+++"):
			class = "add"
		case strings.HasPrefix(line, diffRemoved) && !strings.HasPrefix(line, "---"):
			class = "del"
		}
		if class == "" {
			lines[index] = escape(line)
			continue
		}
		lines[index] = fmt.Sprintf(`<span class="%s">%s</span>`, class, escape(line))
	}
	return strings.Join(lines, "\n")
}
