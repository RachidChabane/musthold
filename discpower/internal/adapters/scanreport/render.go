package scanreport

import (
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
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
// that verifies nothing, and the draw.
func Text(report scan.Report) string {
	var out strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&out, format+newline, args...) }
	line("%s", header(report))
	line("%s%s", indent, strings.Join(split(report), countGap))
	if reasons := unprovenReasons(report); reasons != "" {
		line("%sunproven: %s", indent, reasons)
	}
	line("%s%s", indent, notAPass)
	if report.Partial {
		line("%s%s", indent, partialNote)
	}
	for _, finding := range report.Findings {
		line("")
		line(rankFormat+findingFormat, finding.Rank, finding.Test, finding.Changes)
		line("%s%s", blockIndent, changeLine(finding))
		line("%sproven by: %s", blockIndent, provenBy(finding.Route))
		line("%srepro: %s/%s", blockIndent, FindingDir(report, finding), reproFile)
	}
	line("%s", footer(report))
	return out.String()
}

// Markdown renders the report a pull request comment or `report.md` holds.
func Markdown(report scan.Report) string {
	var out strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&out, format+newline, args...) }
	line("## spec-proof scan")
	line("")
	line("%s", header(report))
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
	line("%s", notAPass)
	if report.Partial {
		line("")
		line("%s.", strings.ToUpper(partialNote[:1])+partialNote[1:])
	}
	for _, finding := range report.Findings {
		line("")
		line("### %d. `%s` "+findingFormat, finding.Rank, finding.Test, finding.Changes)
		line("")
		line("- Change: `%s`", changeLine(finding))
		line("- Proven by: %s", provenBy(finding.Route))
		line("- Reproduce: `%s/%s`", FindingDir(report, finding), reproFile)
		line("")
		line("```diff")
		for _, file := range finding.Files {
			out.WriteString(UnifiedDiff(file.Path, file.Before, file.After))
		}
		line("```")
	}
	line("")
	line("%s", footer(report))
	return out.String()
}
