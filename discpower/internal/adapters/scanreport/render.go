package scanreport

import (
	"fmt"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/scan"
)

// Layout of the terminal headline.
const (
	indent      = "  "
	blockIndent = "   "
	countGap    = "   "
	rankFormat  = "#%d %s "
)

// Text renders the headline a terminal prints: the suite, the four counts over
// their denominators, why the unproven tests are unproven, one block per test
// that verifies nothing, the draw and, when a test verifies nothing, what its
// green leaves unshown.
func Text(report scan.Report) string {
	var out strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&out, format+newline, args...) }
	line("%s", header(report))
	line("%s%s", indent, depthLine(report))
	line("%s%s", indent, strings.Join(split(report), countGap))
	if reasons := unprovenReasons(report); reasons != "" {
		line("%sunproven: %s", indent, reasons)
	}
	if gaps := unprovenGaps(report); gaps != "" {
		line("%sthey lack: %s", indent, gaps)
	}
	line("%s%s", indent, notAPass)
	if report.Partial {
		line("%s%s", indent, partialNote)
	}
	for _, stopped := range notStartedLines(report) {
		line("%s%s", indent, stopped)
	}
	for _, finding := range report.Findings {
		line("")
		line(rankFormat+"%s", finding.Rank, finding.Test, findingSentence(finding))
		line("%s%s", blockIndent, changeLine(finding.Site, finding.Files, finding.Route))
		line("%sproven by: %s", blockIndent, provenBy(finding.Route))
		line("%srepro: %s/%s", blockIndent, FindingDir(report, finding), reproFile)
		if breaks := mayBreak(report, finding); breaks != "" {
			line("%smay break: %s", blockIndent, breaks)
		}
	}
	if report.Promises != nil {
		writeTextPromises(&out, report)
	}
	line("%s", footer(report))
	if closing := stake(report); closing != "" {
		line("%s", closing)
	}
	return out.String()
}

// Markdown renders the report a pull request comment or `report.md` holds.
func Markdown(report scan.Report) string {
	var out strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&out, format+newline, args...) }
	line("## musthold scan")
	line("")
	line("%s", header(report))
	line("")
	line("%s", depthLine(report))
	line("")
	line("| Tests | Count |")
	line("| --- | --- |")
	for _, count := range split(report) {
		number, label, _ := strings.Cut(count, " ")
		line("| %s | %s |", label, number)
	}
	line("")
	if reasons := unprovenReasons(report); reasons != "" {
		line("Unproven: %s.", reasons)
		line("")
	}
	if gaps := unprovenGaps(report); gaps != "" {
		line("They lack: %s.", gaps)
		line("")
		for _, row := range gapTable(report) {
			line("%s", row)
		}
		line("")
	}
	line("%s", notAPass)
	if report.Partial {
		line("")
		line("%s.", strings.ToUpper(partialNote[:1])+partialNote[1:])
	}
	if stopped := notStartedLines(report); len(stopped) > 0 {
		line("")
		for _, entry := range stopped {
			line("- %s.", entry)
		}
	}
	for _, finding := range report.Findings {
		line("")
		line("### %d. `%s` %s", finding.Rank, finding.Test, findingSentence(finding))
		line("")
		line("- Change: `%s`", changeLine(finding.Site, finding.Files, finding.Route))
		line("- Proven by: %s", provenBy(finding.Route))
		line("- Reproduce: `%s/%s`", FindingDir(report, finding), reproFile)
		if breaks := mayBreak(report, finding); breaks != "" {
			line("- May break: %s", breaks)
		}
		line("")
		line("```diff")
		for _, file := range finding.Files {
			out.WriteString(UnifiedDiff(file.Path, file.Before, file.After))
		}
		line("```")
	}
	if report.Promises != nil {
		writeMarkdownPromises(&out, report)
	}
	line("")
	line("%s", footer(report))
	if closing := stake(report); closing != "" {
		line("")
		line("%s", closing)
	}
	return out.String()
}

// writeTextPromises writes the promises of a scan that ran with a host: what
// was checked, each broken promise as a block, the questions and what went
// unchecked.
func writeTextPromises(out *strings.Builder, report scan.Report) {
	line := func(format string, args ...any) { fmt.Fprintf(out, format+newline, args...) }
	promises := report.Promises
	sentence, items := promiseStatus(promises)
	line("")
	line("%s", sentence)
	for _, item := range items {
		line("%s%s", indent, item)
	}
	for _, broken := range promises.Broken {
		line("")
		line(brokenRankFormat+" %s", broken.Rank, brokenTitle(broken))
		line("%s%s", blockIndent, brokenChange(broken))
		line("%sproven by: %s", blockIndent, brokenProof(broken))
		line("%srepro: %s/%s", blockIndent, BrokenPromiseDir(report, broken), brokenScript)
	}
	notes := prefixed(questionPrefix, questionLines(promises))
	if notes = append(notes, prefixed(uncheckedPrefix, uncheckedLines(promises))...); len(notes) > 0 {
		line("")
	}
	for _, note := range notes {
		line("%s", note)
	}
}

// prefixed puts a prefix before each line.
func prefixed(prefix string, lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, text := range lines {
		out = append(out, prefix+text)
	}
	return out
}

// writeMarkdownPromises writes the promises of a scan that ran with a host as
// the sections Promises, Broken promises, Questions and Unchecked.
func writeMarkdownPromises(out *strings.Builder, report scan.Report) {
	line := func(format string, args ...any) { fmt.Fprintf(out, format+newline, args...) }
	promises := report.Promises
	section := func(heading string, items []string) {
		if len(items) == 0 {
			return
		}
		line("")
		line("%s", heading)
		line("")
		for _, item := range items {
			line("%s%s", itemPrefix, item)
		}
	}
	sentence, items := promiseStatus(promises)
	if len(items) == 0 {
		items = []string{sentence}
	}
	section(promisesHeading, items)
	if len(promises.Broken) > 0 {
		line("")
		line("%s", brokenHeading)
	}
	for _, broken := range promises.Broken {
		line("")
		line("#### "+brokenMarkdownFormat+" %s", broken.Rank, brokenTitle(broken))
		line("")
		line("- Change: `%s`", brokenChange(broken))
		line("- Proven by: %s", brokenProof(broken))
		line("- Reproduce: `%s/%s`", BrokenPromiseDir(report, broken), brokenScript)
		line("")
		line("```diff")
		for _, file := range broken.Files {
			out.WriteString(UnifiedDiff(file.Path, file.Before, file.After))
		}
		line("```")
	}
	section(questionsHeading, questionLines(promises))
	section(uncheckedHeading, uncheckedLines(promises))
}
