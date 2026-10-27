package analyze

import (
	"math/big"
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/stats"
)

// The arms of the study (PREREG §5).
const (
	ArmControl   = "control"
	ArmTreatment = "treatment"
)

// Arms lists the arms in order.
func Arms() []string { return []string{ArmControl, ArmTreatment} }

// Population rules of PREREG §10.1 and §10.3.
const (
	// MinRunsPerArm is the counted runs each arm needs for a task to enter the
	// comparative analyses; MinTasks the tasks the primary analysis needs.
	MinRunsPerArm = 2
	MinTasks      = 5
	// MinFindingRuns is the treatment runs with a finding block H2 needs.
	MinFindingRuns = 6
)

// GuardBound is the highest false rejection of the treatment arm guard G allows.
var GuardBound = Fraction{K: 1, N: 3}

// halfRate is the rate H2 tests against.
const halfRate = 0.5

// The decisions of H1 and H2.
const (
	DecisionSupported     = "SUPPORTED"
	DecisionRejectionOnly = "REJECTION-ONLY"
	DecisionContradicted  = "CONTRADICTED"
	DecisionInconclusive  = "INCONCLUSIVE"
	DecisionNotSupported  = "NOT SUPPORTED"
)

// The reasons of an INCONCLUSIVE H1, as the conclusion names them.
const (
	ReasonBlocked = "fewer than five tasks, `blocked`"
	ReasonFloor   = "`p_min` > 0.05, too few escapes to test"
	ReasonNeither = "`p_less` and `p_more` above 0.05"
	ReasonFewRuns = "fewer than six runs with a finding block"
)

// Fraction is k of n, written k/n.
type Fraction = facts.Fraction

func frac(k, n int) Fraction { return Fraction{K: k, N: n} }

// Probability is an exact probability with its value.
type Probability struct {
	Exact string  `json:"exact"`
	Value float64 `json:"value"`
}

func probability(p *big.Rat) Probability {
	value, _ := p.Float64()
	return Probability{Exact: p.RatString(), Value: value}
}

// Rate is a proportion with its exact interval.
type Rate struct {
	Fraction Fraction `json:"fraction"`
	Low      float64  `json:"low"`
	High     float64  `json:"high"`
}

func rate(k, n int) Rate {
	interval := stats.ClopperPearson(k, n)
	return Rate{Fraction: frac(k, n), Low: interval.Low, High: interval.High}
}

// byArm splits runs by arm.
func byArm(runs []facts.Facts) map[string][]facts.Facts {
	split := map[string][]facts.Facts{}
	for _, run := range runs {
		split[run.Arm] = append(split[run.Arm], run)
	}
	return split
}

// byTask splits runs by task, and lists the tasks sorted.
func byTask(runs []facts.Facts) (map[string][]facts.Facts, []string) {
	split := map[string][]facts.Facts{}
	for _, run := range runs {
		split[run.Task] = append(split[run.Task], run)
	}
	tasks := make([]string, 0, len(split))
	for task := range split {
		tasks = append(tasks, task)
	}
	slices.Sort(tasks)
	return split, tasks
}

func count(runs []facts.Facts, holds func(facts.Facts) bool) int {
	n := 0
	for _, run := range runs {
		if holds(run) {
			n++
		}
	}
	return n
}

func filter(runs []facts.Facts, keep func(facts.Facts) bool) []facts.Facts {
	var kept []facts.Facts
	for _, run := range runs {
		if keep(run) {
			kept = append(kept, run)
		}
	}
	return kept
}

// Eligible splits the tasks into those with at least two counted runs in each
// arm, which enter the comparative analyses, and the others.
func Eligible(runs []facts.Facts) (included, excluded []string) {
	split, tasks := byTask(runs)
	for _, task := range tasks {
		arms := byArm(split[task])
		if len(arms[ArmControl]) >= MinRunsPerArm && len(arms[ArmTreatment]) >= MinRunsPerArm {
			included = append(included, task)
		} else {
			excluded = append(excluded, task)
		}
	}
	return included, excluded
}

func inTasks(runs []facts.Facts, tasks []string) []facts.Facts {
	return filter(runs, func(run facts.Facts) bool { return slices.Contains(tasks, run.Task) })
}

// TaskRow is one task's events per arm.
type TaskRow struct {
	Task      string   `json:"task"`
	Control   Fraction `json:"control"`
	Treatment Fraction `json:"treatment"`
}

// Permutation is the stratified permutation test of an indicator.
type Permutation struct {
	PLess     Probability `json:"pLess"`
	PMore     Probability `json:"pMore"`
	PMin      Probability `json:"pMin"`
	Control   Rate        `json:"control"`
	Treatment Rate        `json:"treatment"`
	// RiskDifference is the treatment rate minus the control rate.
	RiskDifference float64   `json:"riskDifference"`
	PerTask        []TaskRow `json:"perTask"`
}

// permutation runs the test of PREREG §10.2 on the indicator, over the runs of
// the tasks.
func permutation(runs []facts.Facts, tasks []string, event func(facts.Facts) bool) (Permutation, stats.Permutation) {
	split, _ := byTask(runs)
	var strata []stats.Stratum
	var test Permutation
	totals := map[string]Fraction{}
	for _, task := range tasks {
		arms := byArm(split[task])
		control, treatment := arms[ArmControl], arms[ArmTreatment]
		row := TaskRow{Task: task, Control: frac(count(control, event), len(control)), Treatment: frac(count(treatment, event), len(treatment))}
		test.PerTask = append(test.PerTask, row)
		strata = append(strata, stats.Stratum{ControlRuns: row.Control.N, ControlEvents: row.Control.K, TreatmentRuns: row.Treatment.N, TreatmentEvents: row.Treatment.K})
		for arm, value := range map[string]Fraction{ArmControl: row.Control, ArmTreatment: row.Treatment} {
			total := totals[arm]
			totals[arm] = frac(total.K+value.K, total.N+value.N)
		}
	}
	exact := stats.StratifiedPermutation(strata)
	test.PLess, test.PMore, test.PMin = probability(exact.PLess), probability(exact.PMore), probability(exact.PMin)
	test.Control = rate(totals[ArmControl].K, totals[ArmControl].N)
	test.Treatment = rate(totals[ArmTreatment].K, totals[ArmTreatment].N)
	test.RiskDifference = totals[ArmTreatment].Float() - totals[ArmControl].Float()
	return test, exact
}

// Guard is guard G of PREREG §10.2.
type Guard struct {
	// FalseRejection is the treatment runs not accepted among those whose change
	// is not defective.
	FalseRejection Fraction `json:"falseRejection"`
	Bound          Fraction `json:"bound"`
	Holds          bool     `json:"holds"`
}

func guard(treatment []facts.Facts) Guard {
	clean := filter(treatment, func(run facts.Facts) bool { return !run.Defective })
	rejected := count(clean, func(run facts.Facts) bool { return !run.Accepted })
	g := Guard{FalseRejection: frac(rejected, len(clean)), Bound: GuardBound}
	g.Holds = len(clean) > 0 && rejected*GuardBound.N <= GuardBound.K*len(clean)
	return g
}

// H1 is the primary endpoint.
type H1 struct {
	Tasks    []string `json:"tasks"`
	Excluded []string `json:"excluded"`
	Decision string   `json:"decision"`
	// Reason names why an INCONCLUSIVE decision was reached.
	Reason string `json:"reason,omitempty"`
	Permutation
	Guard Guard `json:"guard"`
}

// DecideH1 runs the primary analysis on counted runs.
func DecideH1(runs []facts.Facts) H1 {
	included, excluded := Eligible(runs)
	population := inTasks(runs, included)
	test, exact := permutation(population, included, func(run facts.Facts) bool { return run.Escaped })
	h1 := H1{Tasks: orNone(included), Excluded: orNone(excluded), Permutation: test, Guard: guard(byArm(population)[ArmTreatment])}
	switch {
	case len(included) < MinTasks:
		h1.Decision, h1.Reason = DecisionInconclusive, ReasonBlocked
	case stats.Significant(exact.PLess) && h1.Guard.Holds:
		h1.Decision = DecisionSupported
	case stats.Significant(exact.PLess):
		h1.Decision = DecisionRejectionOnly
	case stats.Significant(exact.PMore):
		h1.Decision = DecisionContradicted
	case !stats.Significant(exact.PMin):
		h1.Decision, h1.Reason = DecisionInconclusive, ReasonFloor
	default:
		h1.Decision, h1.Reason = DecisionInconclusive, ReasonNeither
	}
	return h1
}

func orNone(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// H2 is the key secondary endpoint.
type H2 struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
	// Corrected is the treatment runs whose last finding block is corrected,
	// over those with a finding block.
	Corrected Rate        `json:"corrected"`
	P         Probability `json:"p"`
	// OnTarget and Strengthened are over the corrected runs; Left over the runs
	// with a finding block; StrengthenedUnknown counts the corrected runs whose
	// kill shares are missing.
	OnTarget            Fraction `json:"onTarget"`
	Strengthened        Fraction `json:"strengthened"`
	StrengthenedUnknown int      `json:"strengthenedUnknown"`
	Left                Fraction `json:"left"`
	// Traceability is, per arm, the runs whose last traceability block is
	// corrected over the runs with a traceability block.
	Traceability map[string]Fraction `json:"traceability"`
}

// lastBlocks returns the runs' last blocks of the kind.
func lastBlocks(runs []facts.Facts, kind facts.BlockKind) []facts.Block {
	var found []facts.Block
	for _, run := range runs {
		if block, ok := run.LastBlock(kind); ok {
			found = append(found, block)
		}
	}
	return found
}

func countBlocks(blocks []facts.Block, holds func(facts.Block) bool) int {
	n := 0
	for _, block := range blocks {
		if holds(block) {
			n++
		}
	}
	return n
}

// DecideH2 runs the analysis of PREREG §10.3 on counted runs.
func DecideH2(runs []facts.Facts) H2 {
	arms := byArm(runs)
	last := lastBlocks(arms[ArmTreatment], facts.KindFinding)
	corrected := countBlocks(last, func(b facts.Block) bool { return b.Corrected })
	h2 := H2{Corrected: rate(corrected, len(last)), P: probability(stats.BinomialUpper(corrected, len(last))), Traceability: map[string]Fraction{}}
	h2.OnTarget = frac(countBlocks(last, func(b facts.Block) bool { return b.Corrected && b.OnTarget }), corrected)
	h2.Strengthened = frac(countBlocks(last, func(b facts.Block) bool { return b.Corrected && b.Strengthened != nil && *b.Strengthened }), corrected)
	h2.StrengthenedUnknown = countBlocks(last, func(b facts.Block) bool { return b.Corrected && b.Strengthened == nil })
	h2.Left = frac(countBlocks(last, func(b facts.Block) bool { return b.Left }), len(last))
	for _, arm := range Arms() {
		traced := lastBlocks(arms[arm], facts.KindTraceability)
		h2.Traceability[arm] = frac(countBlocks(traced, func(b facts.Block) bool { return b.Corrected }), len(traced))
	}
	exact := stats.BinomialUpper(corrected, len(last))
	switch {
	case len(last) < MinFindingRuns:
		h2.Decision, h2.Reason = DecisionInconclusive, ReasonFewRuns
	case stats.Significant(exact):
		h2.Decision = DecisionSupported
	case h2.Corrected.High < halfRate:
		h2.Decision = DecisionNotSupported
	default:
		h2.Decision = DecisionInconclusive
	}
	return h2
}

// S2 is the test of the killed violations.
type S2 struct {
	Tasks []string `json:"tasks"`
	// Observed is the treatment arm's killed violations; P the probability of at
	// least that many under the null.
	Observed int         `json:"observed"`
	P        Probability `json:"p"`
	// MissingShare counts the runs left out for want of a final kill share.
	MissingShare int  `json:"missingShare"`
	Sign         Sign `json:"sign"`
}

// Sign is an exact one-sided sign test over tasks.
type Sign struct {
	Favourable  int         `json:"favourable"`
	Informative int         `json:"informative"`
	P           Probability `json:"p"`
}

func sign(differences []*big.Rat) Sign {
	test := stats.SignTest(differences)
	return Sign{Favourable: test.Favourable, Informative: test.Informative, P: probability(test.P)}
}

// DecideS2 runs the analysis of PREREG §10.4 S2 on counted runs.
func DecideS2(runs []facts.Facts) S2 {
	included, _ := Eligible(runs)
	population := inTasks(runs, included)
	withShare := filter(population, func(run facts.Facts) bool { return run.Strength.Final != nil })
	split, _ := byTask(withShare)
	var tasks []stats.Valued
	var differences []*big.Rat
	for _, task := range included {
		arms := byArm(split[task])
		valued := stats.Valued{Control: killed(arms[ArmControl]), Treatment: killed(arms[ArmTreatment])}
		tasks = append(tasks, valued)
		differences = append(differences, new(big.Rat).Sub(meanShare(arms[ArmTreatment]), meanShare(arms[ArmControl])))
	}
	exact := stats.StratifiedSum(tasks)
	return S2{
		Tasks: orNone(included), Observed: exact.Observed, P: probability(exact.PMore),
		MissingShare: len(population) - len(withShare), Sign: sign(differences),
	}
}

func killed(runs []facts.Facts) []int {
	values := make([]int, 0, len(runs))
	for _, run := range runs {
		values = append(values, run.Strength.Final.K)
	}
	return values
}

// meanShare is the mean kill share of the runs; 0 with no run.
func meanShare(runs []facts.Facts) *big.Rat {
	total := new(big.Rat)
	if len(runs) == 0 {
		return total
	}
	for _, run := range runs {
		total.Add(total, run.Strength.Final.Rat())
	}
	return total.Quo(total, big.NewRat(int64(len(runs)), 1))
}

// shareDifferences is, per included task, the control arm's share of the event
// minus the treatment arm's.
func shareDifferences(runs []facts.Facts, event func(facts.Facts) bool) []*big.Rat {
	included, _ := Eligible(runs)
	split, _ := byTask(runs)
	var differences []*big.Rat
	for _, task := range included {
		arms := byArm(split[task])
		control := frac(count(arms[ArmControl], event), len(arms[ArmControl]))
		treatment := frac(count(arms[ArmTreatment], event), len(arms[ArmTreatment]))
		differences = append(differences, new(big.Rat).Sub(control.Rat(), treatment.Rat()))
	}
	return differences
}

// majorityDifferences is, per task with a treatment run with a finding block,
// the corrected runs minus the others, as a sign of the per-task majority.
func majorityDifferences(runs []facts.Facts) []*big.Rat {
	split, tasks := byTask(byArm(runs)[ArmTreatment])
	var differences []*big.Rat
	for _, task := range tasks {
		last := lastBlocks(split[task], facts.KindFinding)
		if len(last) == 0 {
			continue
		}
		corrected := countBlocks(last, func(b facts.Block) bool { return b.Corrected })
		differences = append(differences, big.NewRat(int64(2*corrected-len(last)), 1))
	}
	return differences
}

// Core is H1, H2, S1 and S2 on one population of runs.
type Core struct {
	Runs int         `json:"runs"`
	H1   H1          `json:"h1"`
	H2   H2          `json:"h2"`
	S1   Permutation `json:"s1"`
	S2   S2          `json:"s2"`
}

// core runs H1, H2, S1 and S2 on counted runs.
func core(runs []facts.Facts) Core {
	included, _ := Eligible(runs)
	s1, _ := permutation(inTasks(runs, included), included, func(run facts.Facts) bool { return run.Defective })
	return Core{Runs: len(runs), H1: DecideH1(runs), H2: DecideH2(runs), S1: s1, S2: DecideS2(runs)}
}
