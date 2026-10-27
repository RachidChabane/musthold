package analyze

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/conclusion"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// ExploratoryMarker is the comment that holds the place of the exploratory
// discussion, which a person writes in at most 300 words.
const ExploratoryMarker = "<!-- exploratory discussion: at most 300 words, written by hand -->"

// Display precision of the report's numbers.
const (
	probabilityFormat = "%.4f"
	boundFormat       = "%.3f"
	numberFormat      = "%.2f"
	none              = "none"
	notRecorded       = "not recorded in `inputs.json`"
	dash              = "-"
)

// report writes REPORT.md.
type report struct{ b strings.Builder }

func (r *report) line(format string, args ...any) {
	fmt.Fprintf(&r.b, format, args...)
	r.b.WriteString("\n")
}

func (r *report) blank() { r.b.WriteString("\n") }

func (r *report) heading(title string) {
	r.blank()
	r.line("## %s", title)
	r.blank()
}

// Render renders REPORT.md from the summary, in the order of PREREG §11.2. It
// writes no fraction that the summary does not hold.
func Render(s Summary) string {
	r := &report{}
	r.line("# loop-1 results")
	r.provenance(s)
	r.conclusions(s)
	r.h1(s)
	r.h2(s)
	r.secondary(s)
	r.sensitivity(s)
	r.runs(s)
	r.deviations(s)
	r.heading("Limits")
	r.line("%s", conclusion.Limits)
	r.heading("Exploratory discussion (exploratory)")
	r.line("%s", ExploratoryMarker)
	return r.b.String()
}

func code(text string) string { return "`" + text + "`" }

func codes(values []string) string {
	if len(values) == 0 {
		return none
	}
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = code(value)
	}
	return strings.Join(quoted, ", ")
}

func (r *report) provenance(s Summary) {
	r.heading("Provenance")
	for _, tag := range s.Provenance.Tags {
		at := "no token"
		if tag.Time != nil {
			at = tag.Time.UTC().Format(time.RFC3339)
		}
		r.line("- tag %s: commit %s, RFC 3161 time %s", code(tag.Name), code(tag.Commit), at)
	}
	r.line("- `inputs.json` SHA-256: %s", code(s.Provenance.InputsDigest))
	r.line("- tasks: %s", codes(s.Provenance.Tasks))
	switch {
	case s.Provenance.Between == nil:
		r.line("- commits between the tags: %s", notRecorded)
	default:
		r.line("- commits between the tags: %s", codes(s.Provenance.Between))
	}
	switch {
	case s.Provenance.Dropped == nil:
		r.line("- dropped domains: %s", notRecorded)
	case len(s.Provenance.Dropped) == 0:
		r.line("- dropped domains: none")
	}
	for _, dropped := range s.Provenance.Dropped {
		r.line("- dropped domain %s: %s", code(dropped.Domain), code(dropped.Reason))
	}
	r.line("- run phase: first start %s, calendar cap %s, cap reached: %v",
		s.Window.FirstStart.UTC().Format(time.RFC3339), s.Window.Cap.UTC().Format(time.RFC3339), s.Window.CapReached)
}

// H1Conclusion fills the conclusion template of H1's decision.
func H1Conclusion(h1 H1) string {
	values := map[string]string{
		"T":   fmt.Sprint(len(h1.Tasks)),
		"n_c": fmt.Sprint(h1.Control.Fraction.N), "n_t": fmt.Sprint(h1.Treatment.Fraction.N),
		"k_c": fmt.Sprint(h1.Control.Fraction.K), "k_t": fmt.Sprint(h1.Treatment.Fraction.K),
		"r": fmt.Sprint(h1.Guard.FalseRejection.K), "m": fmt.Sprint(h1.Guard.FalseRejection.N),
		"p": fmt.Sprintf(probabilityFormat, h1.PLess.Value), "reason": h1.Reason,
	}
	switch h1.Decision {
	case DecisionSupported:
		return conclusion.Fill(conclusion.H1Supported, values)
	case DecisionRejectionOnly:
		return conclusion.Fill(conclusion.H1RejectionOnly, values)
	case DecisionContradicted:
		values["p"] = fmt.Sprintf(probabilityFormat, h1.PMore.Value)
		return conclusion.Fill(conclusion.H1Contradicted, values)
	default:
		return conclusion.Fill(conclusion.H1Inconclusive, values)
	}
}

// H2Conclusion fills the conclusion template of H2.
func H2Conclusion(h2 H2) string {
	return conclusion.Fill(conclusion.H2, map[string]string{
		"c": fmt.Sprint(h2.Corrected.Fraction.K), "b": fmt.Sprint(h2.Corrected.Fraction.N),
		"lo": fmt.Sprintf(boundFormat, h2.Corrected.Low), "hi": fmt.Sprintf(boundFormat, h2.Corrected.High),
		"decision": h2.Decision,
	})
}

func (r *report) conclusions(s Summary) {
	r.heading("Conclusions")
	r.line("%s", H1Conclusion(s.Analysis.H1))
	r.blank()
	r.line("%s", H2Conclusion(s.Analysis.H2))
}

// population writes the counted runs, the void attempts and the runs without a
// counted attempt, per arm and per cause, and the scope of each protocol
// deviation that touches the runs.
func (r *report) population(s Summary, protocol []string) {
	for _, arm := range Arms() {
		r.line("- %s: %d counted runs; void attempts: %s; runs without a counted attempt: %s",
			arm, s.Population.Counted[arm], causes(s.Population.Void[arm]), causes(s.Population.Missing[arm]))
	}
	r.line("- protocol deviations whose scope touches these runs: %d, scopes: %s", len(protocol), codes(protocol))
}

func causes(counts map[study.Outcome]int) string {
	var parts []string
	for _, outcome := range slices.Sorted(maps.Keys(counts)) {
		if counts[outcome] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", outcome, counts[outcome]))
		}
	}
	if len(parts) == 0 {
		return none
	}
	return strings.Join(parts, ", ")
}

func p(value Probability) string {
	return fmt.Sprintf(probabilityFormat+" (%s)", value.Value, code(value.Exact))
}

func interval(rate Rate) string {
	return fmt.Sprintf("%s ["+boundFormat+", "+boundFormat+"]", rate.Fraction, rate.Low, rate.High)
}

func (r *report) h1(s Summary) {
	h1 := s.Analysis.H1
	r.heading("H1, the primary endpoint")
	r.population(s, s.Analysis.ProtocolDeviations)
	r.blank()
	r.line("- decision: %s", h1.Decision)
	if h1.Reason != "" {
		r.line("- reason: %s", h1.Reason)
	}
	r.line("- tasks in the analysis: %s; tasks left out: %s", codes(h1.Tasks), codes(h1.Excluded))
	r.line("- `p_less` %s, `p_more` %s, `p_min` %s", p(h1.PLess), p(h1.PMore), p(h1.PMin))
	r.line("- guard G: false rejection of the treatment arm %s, bound %s, holds: %v", h1.Guard.FalseRejection, h1.Guard.Bound, h1.Guard.Holds)
	r.effect(h1.Permutation, "escapes")
}

// effect writes the effect table of a permutation test.
func (r *report) effect(test Permutation, event string) {
	r.blank()
	r.line("| Arm | Runs with %s | Exact interval, level 0.95 |", event)
	r.line("|---|---|---|")
	r.line("| control | %s |", strings.Replace(interval(test.Control), " [", " | [", 1))
	r.line("| treatment | %s |", strings.Replace(interval(test.Treatment), " [", " | [", 1))
	r.blank()
	r.line("Risk difference, treatment minus control: "+boundFormat+".", test.RiskDifference)
	r.blank()
	r.line("| Task | Control | Treatment |")
	r.line("|---|---|---|")
	for _, row := range test.PerTask {
		r.line("| %s | %s | %s |", code(row.Task), row.Control, row.Treatment)
	}
}

func (r *report) h2(s Summary) {
	h2 := s.Analysis.H2
	r.heading("H2, the key secondary endpoint")
	r.population(s, s.Analysis.ProtocolDeviations)
	r.blank()
	r.line("- decision: %s", h2.Decision)
	if h2.Reason != "" {
		r.line("- reason: %s", h2.Reason)
	}
	r.line("- corrected treatment runs: %s, exact one-sided binomial p %s", interval(h2.Corrected), p(h2.P))
	r.line("- on target: %s of the corrected runs", h2.OnTarget)
	r.line("- strengthened: %s of the corrected runs, %d with a kill share missing", h2.Strengthened, h2.StrengthenedUnknown)
	r.line("- left: %s of the runs with a finding block", h2.Left)
	for _, arm := range Arms() {
		r.line("- correction of the last traceability block, %s: %s runs", arm, h2.Traceability[arm])
	}
}

func (r *report) secondary(s Summary) {
	r.heading("Secondary endpoints (unadjusted)")
	r.population(s, s.Analysis.ProtocolDeviations)
	r.blank()
	s1 := s.Analysis.S1
	r.line("**S1 defective final change.** `p_less` %s, `p_more` %s, `p_min` %s.", p(s1.PLess), p(s1.PMore), p(s1.PMin))
	r.effect(s1, "a defective change")
	r.blank()
	s2 := s.Analysis.S2
	r.line("**S2 test strength.** Treatment total of killed violations %d, probability of at least that total %s; %d runs without a final kill share. Sign test over tasks on the mean kill share: %d favourable of %d informative tasks, p %s.",
		s2.Observed, p(s2.P), s2.MissingShare, s2.Sign.Favourable, s2.Sign.Informative, p(s2.Sign.P))
	r.blank()
	r.line("**S3 yield and false rejection.**")
	r.blank()
	r.line("| Arm | Yield | False rejection |")
	r.line("|---|---|---|")
	for _, arm := range Arms() {
		r.line("| %s | %s | %s |", arm, interval(s.Secondary.S3[arm].Yield), interval(s.Secondary.S3[arm].FalseRejection))
	}
	r.s4(s)
	r.s5(s)
	r.s6(s)
	r.s7(s)
	r.s8(s)
}

func (r *report) s4(s Summary) {
	r.blank()
	r.line("**S4 blocks.**")
	r.blank()
	r.line("| Arm | Counted runs | Finding blocks | Pending blocks | Traceability blocks | Runs with a finding block | Give-ups | Finding codes |")
	r.line("|---|---|---|---|---|---|---|---|")
	for _, arm := range Arms() {
		s4 := s.Secondary.S4[arm]
		var findings []string
		for _, findingCode := range slices.Sorted(maps.Keys(s4.FindingCodes)) {
			findings = append(findings, fmt.Sprintf("%s %d", code(findingCode), s4.FindingCodes[findingCode]))
		}
		if len(findings) == 0 {
			findings = []string{none}
		}
		r.line("| %s | %d | %d | %d | %d | %s | %d | %s |", arm, s4.Runs, s4.Blocks[facts.KindFinding], s4.Blocks[facts.KindPending],
			s4.Blocks[facts.KindTraceability], interval(s4.RunsWith[facts.KindFinding]), s4.GiveUps, strings.Join(findings, ", "))
	}
}

func (r *report) s5(s Summary) {
	r.blank()
	r.line("**S5 cheating attempts**, runs with a hit over counted runs, with exact intervals at level 0.95.")
	r.blank()
	r.line("| Detector | Control | Treatment |")
	r.line("|---|---|---|")
	for _, detector := range study.Detectors() {
		r.line("| %s | %s | %s |", detector, interval(s.Secondary.S5[ArmControl][detector]), interval(s.Secondary.S5[ArmTreatment][detector]))
	}
}

func (r *report) s6(s Summary) {
	r.blank()
	r.line("**S6 cost.** Dollars are notional: the token is a subscription.")
	r.blank()
	r.line("| Arm | Measure | Median per run | Maximum per run | Per accepted change |")
	r.line("|---|---|---|---|---|")
	for _, arm := range Arms() {
		for _, row := range s.Secondary.S6.PerArm[arm] {
			perAccepted := dash
			if row.PerAccepted != nil {
				perAccepted = fmt.Sprintf(numberFormat, *row.PerAccepted)
			}
			r.line("| %s | %s | "+numberFormat+" | "+numberFormat+" | %s |", arm, row.Measure, row.Median, row.Max, perAccepted)
		}
	}
	r.blank()
	r.line("Per-task difference of the mean per run, treatment minus control (notional USD for the dollars):")
	r.blank()
	r.line("| Task | %s |", strings.Join(Measures(), " | "))
	r.line("|---|%s", strings.Repeat("---|", len(Measures())))
	for _, row := range s.Secondary.S6.PerTask {
		cells := make([]string, 0, len(Measures()))
		for _, name := range Measures() {
			cells = append(cells, fmt.Sprintf(numberFormat, row.Differences[name]))
		}
		r.line("| %s | %s |", code(row.Task), strings.Join(cells, " | "))
	}
}

func (r *report) s7(s Summary) {
	r.blank()
	r.line("**S7 voluntary proofs in the control arm.** Control runs with a voluntary proof: %s.", interval(s.Secondary.S7.Runs))
	for _, entry := range s.Secondary.S7.List {
		r.line("- %s: calls %d, findings printed: %s", code(entry.Run), entry.Calls, codes(entry.Findings))
	}
}

func (r *report) s8(s Summary) {
	r.blank()
	r.line("**S8 per-criterion defects**, runs that violate the criterion over counted runs, with exact intervals at level 0.95.")
	r.blank()
	if len(s.Secondary.S8) == 0 {
		r.line("No criterion is violated in any counted run.")
		return
	}
	r.line("| Task | Criterion | Control | Treatment |")
	r.line("|---|---|---|---|")
	for _, row := range s.Secondary.S8 {
		r.line("| %s | %s | %s | %s |", code(row.Task), code(row.Criterion), interval(row.Control), interval(row.Treatment))
	}
}

func (r *report) sensitivity(s Summary) {
	r.heading("Sensitivity analyses (unadjusted)")
	r.population(s, s.Analysis.ProtocolDeviations)
	r.blank()
	r.line("| Population | Runs | H1 | `p_less` | H2 | Corrected | S1 `p_less` | S2 p | Scopes of the protocol deviations that touch it |")
	r.line("|---|---|---|---|---|---|---|---|---|")
	for _, subset := range []struct {
		label, key string
		core       Core
	}{
		{"without " + code(InSampleTask), SubsetWithoutInSample, s.Sensitivity.WithoutInSample},
		{"without the runs with a detector hit", SubsetWithoutHits, s.Sensitivity.WithoutHits},
		{"without the capped runs", SubsetWithoutCapped, s.Sensitivity.WithoutCapped},
	} {
		c := subset.core
		r.line("| %s | %d | %s | "+probabilityFormat+" | %s | %s | "+probabilityFormat+" | "+probabilityFormat+" | %s |",
			subset.label, c.Runs, c.H1.Decision, c.H1.PLess.Value, c.H2.Decision, c.H2.Corrected.Fraction, c.S1.PLess.Value, c.S2.P.Value,
			codes(s.Sensitivity.ProtocolDeviations[subset.key]))
	}
	r.blank()
	for _, test := range []struct {
		label string
		sign  Sign
	}{
		{"H1, per-task difference of escape shares", s.Sensitivity.SignH1},
		{"S1, per-task difference of defect shares", s.Sensitivity.SignS1},
		{"H2, per-task majority of corrected runs", s.Sensitivity.SignH2},
	} {
		r.line("- sign test over tasks, %s: %d favourable of %d informative tasks, p %s", test.label, test.sign.Favourable, test.sign.Informative, p(test.sign.P))
	}
}

func (r *report) runs(s Summary) {
	r.heading("Runs")
	r.line("| Run | Attempts | State | Accepted | Defective | Blocks: finding, pending, traceability, give-ups | Detector hits | Messages | Notional USD |")
	r.line("|---|---|---|---|---|---|---|---|---|")
	for _, entry := range s.Runs {
		attempts := make([]string, 0, len(entry.Attempts))
		for _, attempt := range entry.Attempts {
			attempts = append(attempts, fmt.Sprintf("%d %s", attempt.Number, attempt.Outcome))
		}
		if len(attempts) == 0 {
			attempts = []string{none}
		}
		cells := []string{dash, dash, dash, dash, dash, dash}
		if f := entry.Facts; f != nil {
			hits := 0
			for _, n := range f.Hits {
				hits += n
			}
			usd := dash
			if f.Cost.NotionalUSD != nil {
				usd = fmt.Sprintf(numberFormat, *f.Cost.NotionalUSD)
			}
			cells = []string{
				fmt.Sprint(f.Accepted), fmt.Sprint(f.Defective),
				fmt.Sprintf("%d, %d, %d, %d", f.BlockCounts[facts.KindFinding], f.BlockCounts[facts.KindPending], f.BlockCounts[facts.KindTraceability], f.GiveUps),
				fmt.Sprint(hits), fmt.Sprint(f.Cost.Messages), usd,
			}
		}
		r.line("| %s | %s | %s | %s |", code(entry.Name), strings.Join(attempts, "; "), entry.State, strings.Join(cells, " | "))
	}
}

func (r *report) deviations(s Summary) {
	r.heading("Deviations")
	var parts []string
	for _, kind := range study.Kinds() {
		parts = append(parts, fmt.Sprintf("%s %d", kind, s.Deviations.Counts[kind]))
	}
	r.line("Entries per kind: %s. Every entry is in %s.", strings.Join(parts, ", "), code(s.Deviations.File))
}
