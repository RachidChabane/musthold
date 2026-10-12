// Package scanreport renders a scan for its readers: the terminal headline, the
// Markdown report a pull request can carry, and the finding folders, each with
// the changed code as a patch, a script that reproduces the passing test on it
// and the record a replay re-executes. Its words never call a test's verdict a
// pass and never call a change a defect.
package scanreport

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

// Vocabulary of the reports.
const (
	// ShortCommitLength is how many characters of a commit id a report shows.
	ShortCommitLength = 7
	// Directory is where the reports of a scan live, under the work tree.
	Directory = ".spec-proof/scan"

	reportFile  = "report.md"
	statement   = "scan.json"
	patchFile   = "mutant.patch"
	reproFile   = "repro.sh"
	findingFile = "finding.json"

	arrow         = "→"
	separator     = " · "
	notAPass      = "Not a pass: unproven and unsampled tests are unjudged."
	noModel       = "no model used"
	noPromises    = "promises not checked"
	partialNote   = "partial: the budget stopped the scan before every mutant ran"
	findingFormat = "passes on all %d proven changes in lines it ran"
	// notStartedMore follows the sentence of a finding when changes in the
	// lines it ran stopped the test from starting.
	notStartedMore = "; %d more stopped it from starting"
	notStartedOne  = "%s: 1 proven change in lines it ran stopped it from starting; it counts neither way"
	notStartedMany = "%s: %d proven changes in lines it ran stopped it from starting; they count neither way"
	killedFormat   = "%s kills it %s"
	differentialBy = "a differential run of %s, reproduced, on a reference that agrees with itself"
	witnessFormat  = "witness %s fails on it, reproduced"
	raisesFormat   = "raises %s"
	argumentFormat = "%s=%s"
	argumentJoin   = ", "
	emptyToken     = "(nothing)"
	durationUnit   = time.Second
	phasesFormat   = "%s (reference %s, coverage %s, draw %s, mutants %s, confirmation %s)"
	noBudget       = "no budget"
	budgetFormat   = "%s budget"
	sampledFormat  = "%d of %d green tests sampled"
	workersFormat  = "%d %s"
	workerNoun     = "worker"
	workersNoun    = "workers"
	fullHint       = " — run with --full for the whole suite"
	stakeFormat    = "%d/%d sampled tests stay green on every proven change in lines they ran: on those lines, their green is no evidence the code is right."
)

// depthNames name the depths of a scan.
var depthNames = map[scan.Depth]string{scan.DepthQuick: "quick scan", scan.DepthFull: "full scan"}

// countLabels name the classes in the headline, in the order of the split.
var countLabels = []struct {
	class format.ScanClass
	label string
}{
	{format.ScanVerifiesNothing, "verify nothing"},
	{format.ScanCaught, "caught ≥ 1 change"},
	{format.ScanUnproven, "unproven"},
}

// reasonLabels name why tests are unproven.
var reasonLabels = []struct {
	reason format.ScanUnprovenReason
	label  string
}{
	{format.ScanTooFewChanges, fmt.Sprintf("with < %d proven changes or < %d sites", vacuity.MinChanges, vacuity.MinSites)},
	{format.ScanInconclusiveOutcome, "with an inconclusive outcome"},
	{format.ScanKillsSentinel, "failing a behaviour-keeping change"},
}

// allRuns writes "every run of a settled outcome".
var allRuns = fmt.Sprintf("%d/%d", vacuity.RunsPerOutcome, vacuity.RunsPerOutcome)

// ShortCommit is the commit id as a report shows it.
func ShortCommit(commit string) string {
	return commit[:min(len(commit), ShortCommitLength)]
}

// FindingDir is the folder of a finding, relative to the work tree.
func FindingDir(report scan.Report, finding scan.Finding) string {
	return fmt.Sprintf("%s/%s/%d", Directory, ShortCommit(report.Commit), finding.Rank)
}

// header is the first line: repository, commit, runner, suite and draw.
func header(report scan.Report) string {
	parts := []string{
		fmt.Sprintf("%s @ %s", report.Repository, ShortCommit(report.Commit)),
		string(report.Runner),
		fmt.Sprintf("%d tests (%d green %s)", report.Suite.Tests, report.Suite.Green, allRuns),
		fmt.Sprintf("%d sampled", report.Suite.Sampled),
		fmt.Sprintf("seed %d", report.Seed),
	}
	if report.Mode == format.ScanModeChange {
		parts = append(parts, "change since "+ShortCommit(report.Base))
	}
	return strings.Join(parts, separator)
}

// depthLine says how deep the scan went: quick or full, its budget, its time,
// how many green tests it sampled and on how many workers, and, for a quick
// scan, how to judge more.
func depthLine(report scan.Report) string {
	budget := noBudget
	if report.Budget > 0 {
		budget = fmt.Sprintf(budgetFormat, report.Budget)
	}
	parts := []string{
		depthNames[report.Depth], budget, round(report.Elapsed),
		fmt.Sprintf(sampledFormat, report.Suite.Sampled, report.Suite.Green), workersOf(report.Workers),
	}
	text := strings.Join(parts, separator)
	if report.Depth == scan.DepthQuick {
		text += fullHint
	}
	return text
}

// workersOf writes how many workers ran the scan.
func workersOf(workers int) string {
	return fmt.Sprintf(workersFormat, workers, plural(workers, workerNoun, workersNoun))
}

// split is the four counts, each over its denominator: the verdicts over the
// sampled green tests, the unsampled over the green ones.
func split(report scan.Report) []string {
	counts := report.Statement.Predicate.Counts
	var parts []string
	for _, entry := range countLabels {
		parts = append(parts, fmt.Sprintf("%d/%d %s", counts.Of(entry.class), counts.Judged(), entry.label))
	}
	return append(parts, fmt.Sprintf("%d/%d not sampled", counts.NotSampled, counts.NotSampled+counts.Judged()))
}

// unprovenReasons breaks the unproven tests down by reason.
func unprovenReasons(report scan.Report) string {
	byReason := map[format.ScanUnprovenReason]int{}
	for _, test := range report.Statement.Predicate.Tests {
		if test.Class == format.ScanUnproven {
			byReason[test.Reason]++
		}
	}
	var parts []string
	for _, entry := range reasonLabels {
		if byReason[entry.reason] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", byReason[entry.reason], entry.label))
		}
	}
	return strings.Join(parts, argumentJoin)
}

// reasonLabel names why one test is unproven.
func reasonLabel(reason format.ScanUnprovenReason) string {
	for _, entry := range reasonLabels {
		if entry.reason == reason {
			return entry.label
		}
	}
	return string(reason)
}

// elapsed is the time of the scan and of each of its phases.
func elapsed(report scan.Report) string {
	phases := report.Phases
	return fmt.Sprintf(phasesFormat, round(report.Elapsed), round(phases.Reference), round(phases.Coverage), round(phases.Draw), round(phases.Mutants), round(phases.Confirmation))
}

func round(duration time.Duration) string { return duration.Round(durationUnit).String() }

// footer is the last line: the draw, the time, the model and the promises.
func footer(report scan.Report) string {
	mutants := report.Mutants
	parts := []string{fmt.Sprintf("%d mutants (%d proven", mutants.Ran, mutants.Proven)}
	if mutants.CompiledIdentical > 0 {
		parts[0] += fmt.Sprintf(", %d identical when compiled", mutants.CompiledIdentical)
	}
	if mutants.DidNotBuild > 0 {
		parts[0] += fmt.Sprintf(", %d did not build or start", mutants.DidNotBuild)
	}
	if mutants.StoppedSentinels > 0 {
		parts[0] += fmt.Sprintf(stoppedSentinelsFormat, mutants.StoppedSentinels, plural(mutants.StoppedSentinels, sentinelNoun, sentinelsNoun))
	}
	if mutants.NotRun > 0 {
		parts[0] += fmt.Sprintf(notRunFormat, mutants.NotRun, scan.GapNotRun)
	}
	parts[0] += ")"
	parts = append(parts, elapsed(report), noModel, noPromises)
	return strings.Join(parts, separator)
}

// notRunFormat counts the drawn mutants the budget left unrun.
const notRunFormat = ", %d %s"

// Vocabulary of the sentinels that stopped a test binary.
const (
	stoppedSentinelsFormat = ", %d %s stopped a test binary"
	sentinelNoun           = "sentinel"
	sentinelsNoun          = "sentinels"
)

// plural is the noun for a count: the singular for one.
func plural(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
}

// findingSentence is the literal sentence of a finding, with the changes that
// stopped the test from starting when there are some.
func findingSentence(finding scan.Finding) string {
	sentence := fmt.Sprintf(findingFormat, finding.Changes)
	if finding.NotStarted > 0 {
		sentence += fmt.Sprintf(notStartedMore, finding.NotStarted)
	}
	return sentence
}

// notStartedLines say, for each judged test that is not a finding and that
// changes in the lines it ran stopped from starting, how many did, in test id
// order.
func notStartedLines(report scan.Report) []string {
	findings := map[string]bool{}
	for _, finding := range report.Findings {
		findings[string(finding.Test)] = true
	}
	var lines []string
	for _, test := range report.Statement.Predicate.Tests {
		switch count := len(test.NotStarted); {
		case count == 0 || findings[test.ID]:
		case count == 1:
			lines = append(lines, fmt.Sprintf(notStartedOne, test.ID))
		default:
			lines = append(lines, fmt.Sprintf(notStartedMany, test.ID, count))
		}
	}
	return lines
}

// stake is the closing line that says what the tests that verify nothing
// leave unshown, or "" when no sampled test verifies nothing.
func stake(report scan.Report) string {
	counts := report.Statement.Predicate.Counts
	nothing := counts.Of(format.ScanVerifiesNothing)
	if nothing == 0 {
		return ""
	}
	return fmt.Sprintf(stakeFormat, nothing, counts.Judged())
}

// changeLine shows the change of a finding: where, what became what and, for
// a differential route, the input and the two results.
func changeLine(finding scan.Finding) string {
	line := fmt.Sprintf("%s:%d  %s", finding.Site.File, finding.Site.Line, tokenChange(finding))
	if finding.Route.Kind == format.ScanRouteDifferential {
		line += fmt.Sprintf("   input %s: reference %s, mutant %s",
			renderInput(finding.Route.Input), renderObservation(finding.Route.Reference), renderObservation(finding.Route.Mutant))
	}
	return line
}

// provenBy says which executed evidence proves the change.
func provenBy(route format.ScanRoute) string {
	switch route.Kind {
	case format.ScanRouteKilledByTest:
		return fmt.Sprintf(killedFormat, route.Test, allRuns)
	case format.ScanRouteDifferential:
		return fmt.Sprintf(differentialBy, route.Operation)
	}
	return fmt.Sprintf(witnessFormat, route.Witness)
}

// tokenChange is the smallest span of the changed line that differs, as
// `before → after`; a change of several lines shows its first line on each
// side.
func tokenChange(finding scan.Finding) string {
	for _, file := range finding.Files {
		removed, added := changedLines(file.Before, file.After)
		if len(removed) == 0 && len(added) == 0 {
			continue
		}
		before, after := firstOrEmpty(removed), firstOrEmpty(added)
		if len(removed) == 1 && len(added) == 1 {
			before, after = differingSpan(before, after)
		}
		return orEmpty(strings.TrimSpace(before)) + " " + arrow + " " + orEmpty(strings.TrimSpace(after))
	}
	return ""
}

func firstOrEmpty(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func orEmpty(text string) string {
	if text == "" {
		return emptyToken
	}
	return text
}

// differingSpan trims the common prefix and suffix of two lines, widened to
// whole tokens so that `>=` against `>` reads as the two operators.
func differingSpan(before, after string) (string, string) {
	prefix := 0
	for prefix < len(before) && prefix < len(after) && before[prefix] == after[prefix] {
		prefix++
	}
	for prefix > 0 && isTokenByte(before[prefix-1]) {
		prefix--
	}
	suffix := 0
	for suffix < len(before)-prefix && suffix < len(after)-prefix && before[len(before)-1-suffix] == after[len(after)-1-suffix] {
		suffix++
	}
	for suffix > 0 && isTokenByte(before[len(before)-suffix]) {
		suffix--
	}
	return before[prefix : len(before)-suffix], after[prefix : len(after)-suffix]
}

// tokenBreaks end a token: white space, brackets and punctuation.
const tokenBreaks = " \t()[]{},;:"

func isTokenByte(b byte) bool { return !strings.ContainsRune(tokenBreaks, rune(b)) }

// renderInput writes the arguments of a differential input as
// `name=value, ...`, by name.
func renderInput(input string) string {
	var args map[string]spec.Literal
	if json.Unmarshal([]byte(input), &args) != nil {
		return input
	}
	var parts []string
	for _, name := range slices.Sorted(maps.Keys(args)) {
		parts = append(parts, fmt.Sprintf(argumentFormat, name, renderLiteral(args[name].Type, args[name].Value)))
	}
	return strings.Join(parts, argumentJoin)
}

// renderObservation writes what a call returned or raised.
func renderObservation(text string) string {
	var observation spec.Observation
	if json.Unmarshal([]byte(text), &observation) != nil {
		return text
	}
	if observation.Kind == spec.Raises {
		return fmt.Sprintf(raisesFormat, observation.Value)
	}
	return renderLiteral(observation.Type, observation.Value)
}

func renderLiteral(kind spec.LiteralType, value string) string {
	if kind == spec.TypeString {
		return fmt.Sprintf("%q", value)
	}
	return value
}
