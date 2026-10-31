package report

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/conclusion"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/reportlint"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/analysis"
)

// The rules loop-2's lint adds to loop-1's.
const (
	RuleOutcome     reportlint.Rule = "outcome not summary.json's"
	RuleSpecWitness reportlint.Rule = "spec-witness claim not summary.json's"
)

// wholeReport is the line number of a problem that no single line carries.
const wholeReport = 0

// Lint checks the report by loop-1's rules with loop-2's conclusion templates,
// then checks that it states summary.json's outcome and reading, and claims a
// gain owed mainly to the spec-witness check exactly when summary.json does.
// It returns every problem in line order.
func Lint(in reportlint.Input) ([]reportlint.Problem, error) {
	problems, err := reportlint.Linter{Templates: Templates()}.Lint(in)
	if err != nil {
		return nil, err
	}
	var summary analysis.Summary
	if err := json.Unmarshal(in.Summary, &summary); err != nil {
		return nil, fmt.Errorf("summary.json: %w", err)
	}
	lines := strings.Split(in.Report, "\n")
	want := OutcomeLine(summary.Analysis.Primary)
	if !slices.Contains(lines, want) {
		at := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, OutcomeLinePrefix) })
		problems = append(problems, reportlint.Problem{Line: at + 1, Rule: RuleOutcome, Text: want})
	}
	claim := conclusion.Pattern(SpecWitnessGain)
	at := slices.IndexFunc(lines, claim.MatchString)
	mainly := summary.Analysis.Catches.MainlySpecWitness
	switch {
	case at >= 0 && !mainly:
		problems = append(problems, reportlint.Problem{Line: at + 1, Rule: RuleSpecWitness, Text: lines[at]})
	case at < 0 && mainly:
		problems = append(problems, reportlint.Problem{Line: wholeReport, Rule: RuleSpecWitness, Text: SpecWitnessConclusion(summary.Analysis.Catches)})
	}
	slices.SortStableFunc(problems, func(a, b reportlint.Problem) int { return a.Line - b.Line })
	return problems, nil
}
