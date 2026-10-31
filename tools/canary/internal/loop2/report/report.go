// Package report renders results/REPORT.md of the loop-2 study from its
// summary, and lints it: loop-1's rules with loop-2's conclusion templates,
// plus the outcome and the spec-witness claim checked against the summary.
// Render and Lint are pure.
package report

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/conclusion"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/analysis"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/catch"
)

// Display precision and fixed texts of the report.
const (
	boundFormat       = "%.3f"
	probabilityFormat = "%.4f"
	none              = "none"
	noToken           = "no token"
	notGraded         = "-"
	// OutcomeLinePrefix begins the line that states the outcome, which the lint
	// reads back.
	OutcomeLinePrefix = "- outcome: "
)

// OutcomeLine is the line that states the outcome, its row and its reading.
func OutcomeLine(p analysis.Primary) string {
	return fmt.Sprintf("%s**%s**, row %d of ADR 0017's table: %s; %s.", OutcomeLinePrefix, p.Outcome, p.Row, p.Reading, p.Verdict)
}

type writer struct{ b strings.Builder }

func (w *writer) line(format string, args ...any) {
	fmt.Fprintf(&w.b, format, args...)
	w.b.WriteString("\n")
}

func (w *writer) blank() { w.b.WriteString("\n") }

func (w *writer) heading(title string) {
	w.blank()
	w.line("## %s", title)
	w.blank()
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

// Render renders REPORT.md from the summary. It writes no fraction that the
// summary does not hold.
func Render(s analysis.Summary) string {
	w := &writer{}
	a := s.Analysis
	w.line("# loop-2 results")
	w.provenance(s)
	w.conclusions(a)
	w.outcome(s)
	w.primary("Mutation-only effect", a.MutationOnly)
	w.catches(a.Catches)
	w.h2(a.H2)
	w.gate(a.Gate)
	w.readjudication(a.Readjudication)
	w.heading("loop-1's secondary endpoints (unadjusted)")
	w.b.WriteString(analyze.RenderKept(a.Kept))
	w.runs(s)
	w.deviations(s)
	w.heading("Limits")
	w.line("%s A negative result covers its regime: %s.", Limits, Regime)
	w.heading("Exploratory discussion (exploratory)")
	w.line("%s", analyze.ExploratoryMarker)
	return w.b.String()
}

func (w *writer) provenance(s analysis.Summary) {
	w.heading("Provenance")
	for _, tag := range s.Provenance.Tags {
		at := noToken
		if tag.Time != nil {
			at = tag.Time.UTC().Format(time.RFC3339)
		}
		w.line("- tag %s: commit %s, RFC 3161 time %s", code(tag.Name), code(tag.Commit), at)
	}
	w.line("- `inputs.json` SHA-256: %s", code(s.Provenance.InputsDigest))
	w.line("- tasks: %s", codes(s.Provenance.Tasks))
	w.line("- run phase: first start %s, calendar cap %s, cap reached: %v",
		s.Window.FirstStart.UTC().Format(time.RFC3339), s.Window.Cap.UTC().Format(time.RFC3339), s.Window.CapReached)
	for _, arm := range analyze.Arms() {
		w.line("- %s: %d counted runs; void attempts: %s; runs without a counted attempt: %s",
			arm, s.Population.Counted[arm], causes(s.Population.Void[arm]), causes(s.Population.Missing[arm]))
	}
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

// OutcomeConclusion fills the conclusion of the outcome.
func OutcomeConclusion(a analysis.Analysis) string {
	p := a.Primary
	if !a.Feasibility.Runs {
		return conclusion.Fill(CapInconclusive, map[string]string{"reason": p.Reason})
	}
	return conclusion.Fill(Outcome, map[string]string{
		"T":   fmt.Sprint(len(p.Tasks)),
		"n_c": fmt.Sprint(p.Control.Fraction.N), "n_t": fmt.Sprint(p.Treatment.Fraction.N),
		"k_c": fmt.Sprint(p.Control.Fraction.K), "k_t": fmt.Sprint(p.Treatment.Fraction.K),
		"p_less": fmt.Sprintf(probabilityFormat, p.PLess.Value), "p_more": fmt.Sprintf(probabilityFormat, p.PMore.Value),
		"low": fmt.Sprintf(boundFormat, p.Interval.Low), "high": fmt.Sprintf(boundFormat, p.Interval.High),
		"outcome": string(p.Outcome), "reading": p.Reading,
	})
}

// SpecWitnessConclusion fills the sentence of a gain owed mainly to the
// spec-witness check.
func SpecWitnessConclusion(c catch.Summary) string {
	return conclusion.Fill(SpecWitnessGain, map[string]string{"s": fmt.Sprint(c.SpecWitness.K), "c": fmt.Sprint(c.SpecWitness.N)})
}

// H2Conclusion fills the conclusion of H2.
func H2Conclusion(h2 analysis.H2) string {
	return conclusion.Fill(CorrectionAfterH2, map[string]string{
		"c": fmt.Sprint(h2.Corrected.Fraction.K), "b": fmt.Sprint(h2.Corrected.Fraction.N),
		"lo": fmt.Sprintf(boundFormat, h2.Corrected.Low), "hi": fmt.Sprintf(boundFormat, h2.Corrected.High),
		"decision": h2.Decision,
	})
}

func (w *writer) conclusions(a analysis.Analysis) {
	w.heading("Conclusions")
	w.line("%s", OutcomeConclusion(a))
	if a.Catches.MainlySpecWitness {
		w.blank()
		w.line("%s", SpecWitnessConclusion(a.Catches))
	}
	w.blank()
	w.line("%s", H2Conclusion(a.H2))
}

func (w *writer) outcome(s analysis.Summary) {
	w.heading("Outcome")
	w.line("%s", OutcomeLine(s.Analysis.Primary))
	f := s.Analysis.Feasibility
	w.line("- calendar rule: cap reached %v; %d tasks with at least %d counted runs in each arm, %d needed at the cap; the analysis runs: %v",
		f.CapReached, f.Tasks, analysis.CapRunsPerArm, f.Needed, f.Runs)
	w.evidence(s.Analysis.Primary)
}

// primary writes an outcome decided again on other runs.
func (w *writer) primary(title string, p analysis.Primary) {
	w.heading(title)
	w.line("- outcome: %s, row %d: %s", p.Outcome, p.Row, p.Reading)
	w.evidence(p)
}

func (w *writer) evidence(p analysis.Primary) {
	if p.Reason != "" {
		w.line("- reason: %s", p.Reason)
	}
	w.line("- strata: %s; tasks left out: %s", codes(p.Tasks), codes(p.Excluded))
	w.line("- `p_less` %s, `p_more` %s, one-sided level 0.05", analyze.FormatProbability(p.PLess), analyze.FormatProbability(p.PMore))
	w.line("- score at the margin 0.15: %v; 0.90 score interval of the difference, control minus treatment: ["+boundFormat+", "+boundFormat+"]; the interval reads the margin as the score does: %v",
		float64(p.ScoreAtMargin), p.Interval.Low, p.Interval.High, p.IntervalAgrees)
	g, y := p.Guards.G, p.Guards.Y
	w.line("- guard G: refused oracle-clean treatment changes %s, bound %s, holds: %v", g.Refused, g.Bound, g.Holds)
	w.line("- guard Y: runs ending in an accepted oracle-clean change, control %s, treatment %s, largest shortfall %s, holds: %v", y.Control, y.Treatment, y.Bound, y.Holds)
	w.blank()
	w.line("| Arm | Escapes | Exact interval, level 0.95 |")
	w.line("|---|---|---|")
	w.line("| control | %s |", strings.Replace(analyze.FormatRate(p.Control), " [", " | [", 1))
	w.line("| treatment | %s |", strings.Replace(analyze.FormatRate(p.Treatment), " [", " | [", 1))
	w.blank()
	w.line("Risk reduction, control minus treatment: "+boundFormat+".", p.RiskReduction)
	w.blank()
	w.line("| Task | Control | Treatment |")
	w.line("|---|---|---|")
	for _, row := range p.PerTask {
		w.line("| %s | %s | %s |", code(row.Task), row.Control, row.Treatment)
	}
}

func (w *writer) catches(c catch.Summary) {
	w.heading("Catches and attribution")
	w.line("- catches attributed to `spec_witness_failed`: %s; the gain comes mainly from the spec-witness check: %v", c.SpecWitness, c.MainlySpecWitness)
	w.blank()
	w.line("| Attribution | Catches |")
	w.line("|---|---|")
	for _, attribution := range catch.Attributions() {
		w.line("| %s | %d |", code(string(attribution)), c.ByAttribution[attribution])
	}
	w.blank()
	if len(c.Catches) == 0 {
		w.line("No treatment run is a catch.")
		return
	}
	w.line("| Run | Kind | Attribution | Findings |")
	w.line("|---|---|---|---|")
	for _, caught := range c.Catches {
		var findings []string
		for _, finding := range caught.Findings {
			findings = append(findings, finding.Code+" "+finding.Criterion)
		}
		w.line("| %s | %s | %s | %s |", code(caught.Run), caught.Kind, code(string(caught.Attribution)), codes(findings))
	}
}

func (w *writer) h2(h2 analysis.H2) {
	w.heading("H2, correction")
	w.line("- decision: %s", h2.Decision)
	w.line("- corrected and then accepted treatment runs: %s, exact one-sided binomial p %s", analyze.FormatRate(h2.Corrected), analyze.FormatProbability(h2.P))
	w.line("- on target: %s of the corrected runs", h2.OnTarget)
	w.line("- strengthened: %s of the corrected runs, %d with a kill share missing", h2.Strengthened, h2.StrengthenedUnknown)
	w.line("- left: %s of the runs with a finding block", h2.Left)
}

func (w *writer) gate(g analysis.Gate) {
	w.heading("Instructions against the gate")
	w.line("| Arm | Runs with a first-stop grade | Without | Defective at the first stop | Escapes |")
	w.line("|---|---|---|---|---|")
	for _, arm := range analyze.Arms() {
		stops := g.PerArm[arm]
		w.line("| %s | %d | %d | %s | %s |", arm, stops.Graded, stops.Ungraded, analyze.FormatRate(stops.FirstStopDefect), analyze.FormatRate(stops.Escape))
	}
	w.blank()
	w.line("- first-stop defect, exact stratified test: `p_less` %s, `p_more` %s", analyze.FormatProbability(g.PLess), analyze.FormatProbability(g.PMore))
	w.line("- effect of the instructions and in-turn proofs, control minus treatment at the first stop: "+boundFormat, g.Instructions)
	w.line("- effect of the gate, from the first stop to the final change: "+boundFormat, g.GateEffect)
}

func (w *writer) readjudication(r analysis.Readjudication) {
	w.heading("Re-adjudication")
	w.line("- disputed escapes: %s", r.Disputed)
	w.blank()
	if len(r.Escapes) > 0 {
		w.line("| Run | Arm | Criterion | Spec sentence | Verdicts | Disputed |")
		w.line("|---|---|---|---|---|---|")
		for _, escape := range r.Escapes {
			var verdicts []string
			for _, verdict := range escape.Verdicts {
				verdicts = append(verdicts, fmt.Sprintf("%s defective: %v", verdict.Judge, verdict.Defective))
			}
			w.line("| %s | %s | %s | %s | %s | %v |", code(escape.Run), escape.Arm, code(escape.Criterion), escape.Sentence, strings.Join(verdicts, "; "), escape.Disputed)
		}
	}
	w.primary("Sensitivity without the disputed escapes", r.Sensitivity)
}

func (w *writer) runs(s analysis.Summary) {
	w.heading("Runs")
	w.line("| Run | State | Attempts | Accepted | Defective | Defective at the first stop | Refusal | Finding blocks |")
	w.line("|---|---|---|---|---|---|---|---|")
	for _, run := range s.Runs {
		var attempts []string
		for _, attempt := range run.Attempts {
			attempts = append(attempts, fmt.Sprintf("%d %s", attempt.Number, attempt.Outcome))
		}
		accepted, defective, first, refusal, findings := notGraded, notGraded, notGraded, notGraded, notGraded
		if run.Facts != nil {
			accepted, defective = fmt.Sprint(run.Facts.Accepted), fmt.Sprint(run.Facts.Defective)
			findings = fmt.Sprint(run.Facts.BlockCounts[facts.KindFinding])
		}
		if run.FirstStop != nil {
			first = fmt.Sprint(run.FirstStop.Defective)
		}
		if run.Refusal != nil {
			refusal = string(run.Refusal.Cause)
		}
		w.line("| %s | %s | %s | %s | %s | %s | %s | %s |", code(run.Name), run.State, strings.Join(attempts, ", "), accepted, defective, first, refusal, findings)
	}
}

func (w *writer) deviations(s analysis.Summary) {
	w.heading("Deviations")
	var parts []string
	for _, kind := range study.Kinds() {
		parts = append(parts, fmt.Sprintf("%s %d", kind, s.Deviations.Counts[kind]))
	}
	w.line("%s; the entries are in %s.", strings.Join(parts, ", "), code(s.Deviations.File))
}
