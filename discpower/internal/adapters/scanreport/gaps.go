package scanreport

import (
	"fmt"
	"maps"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
)

// Vocabulary of the composition of the unproven tests.
const (
	noMutantFormat  = "%d ran no drawn mutant"
	gapsFormat      = "%d unproven mutants on their lines: %s"
	gapFormat       = "%d %s"
	noWitness       = "no witness without a host"
	gapTableHeading = "| Unproven test | Reason | Proven changes | Unproven mutants on its lines |"
	gapTableRule    = "| --- | --- | --- | --- |"
	gapTableRow     = "| `%s` | %s | %d | %s |"
	noGap           = "none"
)

// unprovenGaps says what the unproven tests lack: how many ran no drawn
// mutant and, over the distinct unproven mutants on their lines, how many
// have each gap; no unproven test has a witness without a host.
func unprovenGaps(report scan.Report) string {
	if len(report.Unproven) == 0 {
		return ""
	}
	var parts []string
	if empty := countEmpty(report.Unproven); empty > 0 {
		parts = append(parts, fmt.Sprintf(noMutantFormat, empty))
	}
	gaps := map[string]scan.Gap{}
	for _, test := range report.Unproven {
		maps.Copy(gaps, test.Gaps)
	}
	if len(gaps) > 0 {
		parts = append(parts, fmt.Sprintf(gapsFormat, len(gaps), gapCounts(gaps)))
	}
	return strings.Join(append(parts, noWitness), separator)
}

func countEmpty(tests []scan.UnprovenTest) int {
	empty := 0
	for _, test := range tests {
		if test.Proven == 0 && len(test.Gaps) == 0 {
			empty++
		}
	}
	return empty
}

// gapCounts writes how many mutants have each gap, in the order of scan.Gaps.
func gapCounts(gaps map[string]scan.Gap) string {
	counts := map[scan.Gap]int{}
	for _, gap := range gaps {
		counts[gap]++
	}
	var parts []string
	for _, gap := range scan.Gaps() {
		if counts[gap] > 0 {
			parts = append(parts, fmt.Sprintf(gapFormat, counts[gap], gap))
		}
	}
	return strings.Join(parts, argumentJoin)
}

// gapTable lists each unproven test with its reason, its proven changes and
// the gaps of the other mutants on its lines.
func gapTable(report scan.Report) []string {
	rows := []string{gapTableHeading, gapTableRule}
	for _, test := range report.Unproven {
		gaps := noGap
		if len(test.Gaps) > 0 {
			gaps = gapCounts(test.Gaps)
		}
		rows = append(rows, fmt.Sprintf(gapTableRow, test.Test, reasonLabel(test.Reason), test.Proven, gaps))
	}
	return rows
}
