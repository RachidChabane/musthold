package power

import (
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/policy"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
)

// Result is the terminal outcome of a criterion (ADR 0003, terminal outcomes).
type Result string

// Terminal outcomes, from the most to the least severe.
const (
	Block    Result = "BLOCK"
	Escalate Result = "ESCALATE"
	Pass     Result = "PASS"
)

// Finding names a ground of an outcome, after the finding categories of Shallnot.
type Finding string

// Findings of a criterion.
const (
	FindingCorrectCodeFails       Finding = "correct_code_fails"
	FindingExampleFailed          Finding = "example_failed"
	FindingSurvivingValid         Finding = "surviving_valid_mutant"
	FindingNoValidMutant          Finding = "no_valid_mutant"
	FindingNoKilledValidMutant    Finding = "no_killed_valid_mutant"
	FindingMutationSensitiveTest  Finding = "mutation_sensitive_test"
	FindingBudgetExhausted        Finding = "budget_exhausted"
	FindingHostUnresponsive       Finding = "host_unresponsive"
	FindingUnexcludedSurvivor     Finding = "unexcluded_survivor"
	FindingExampleExecutorMissing Finding = "example_executor_unavailable"
)

// Entry is one mutant of a pool with the validity votes cast on it.
type Entry struct {
	Facts Facts
	Votes []Vote
	// Unjudged says why a mutant that needed votes has none: the budget ran out
	// or the host did not answer.
	Unjudged Reason
}

// Standing is everything the terminal outcome of one criterion and one test set
// rests on.
type Standing struct {
	Subject           Subject
	CorrectCodePasses bool
	// ExampleFailed is set when an example of the criterion fails on the
	// reference code.
	ExampleFailed bool
	// Witnesses are the witnesses of every criterion, with what they did on the
	// reference and the planted refactors and what the scope judges answered.
	Witnesses []witness.Record
	Entries   []Entry
	// BudgetExhausted and HostUnresponsive are set when a ticket could not be
	// obtained.
	BudgetExhausted  bool
	HostUnresponsive bool
}

// Outcome is the terminal outcome and its findings, sorted.
type Outcome struct {
	Result   Result
	Findings []Finding
}

// Has reports whether the outcome carries the finding.
func (o Outcome) Has(finding Finding) bool { return slices.Contains(o.Findings, finding) }

// Evaluation is the outcome of a criterion with the decision taken for every
// mutant of its pool, by mutant id, and the fault classes of the pool.
type Evaluation struct {
	Outcome   Outcome
	Decisions map[string]Decision
	Faults    []FaultClass
}

// Evaluate applies G3-W to one criterion and one test set, in order, BLOCK
// before ESCALATE before PASS:
//
//  1. traceability is Shallnot's;
//  2. the reference fails a test of the set, or an example of the criterion
//     fails on it: BLOCK;
//  3. an owner-attributed, VALID_e or collateral mutant survives every test of
//     the set: BLOCK;
//  4. no credit-bearing mutant of the pool is killed by the set, an empty pool
//     included: ESCALATE;
//  5. any other survivor that execution did not exclude: ESCALATE;
//  6. alignment is Shallnot's;
//  7. otherwise PASS. Pass credit is a credit-bearing mutant killed by a test of
//     the set that killed no sentinel.
//
// A test of the set that killed a sentinel escalates, and so do an exhausted
// budget and an unresponsive host: the gate that reads the Statement does the
// same.
func Evaluate(standing Standing, rules Rules) Evaluation {
	universe := NewUniverse(standing.Witnesses, rules)
	evaluation := Evaluation{Decisions: map[string]Decision{}}
	var blocks, escalations []Finding
	if !standing.CorrectCodePasses {
		blocks = append(blocks, FindingCorrectCodeFails)
	}
	if standing.ExampleFailed {
		blocks = append(blocks, FindingExampleFailed)
	}
	subject := standing.Subject
	creditBearing, credited := 0, 0
	for _, entry := range standing.Entries {
		decision := DecideEntry(subject, universe, entry, rules)
		evaluation.Decisions[entry.Facts.ID] = decision
		if entry.Facts.Origin == OriginSentinel {
			continue
		}
		survived := subject.AllSurvived(entry.Facts)
		if decision.CreditBearing {
			creditBearing++
			if subject.Killed(entry.Facts) && len(subject.SpecificKillers(entry.Facts)) > 0 {
				credited++
			}
		}
		switch {
		case decision.Carries() && survived:
			blocks = append(blocks, FindingSurvivingValid)
		case decision.Class == ClassUnsettled && survived:
			escalations = append(escalations, FindingUnexcludedSurvivor)
		}
	}
	if standing.CorrectCodePasses && !standing.ExampleFailed {
		escalations = append(escalations, noCreditFindings(creditBearing, credited)...)
	}
	escalations = append(escalations, sensitiveTests(subject)...)
	if standing.BudgetExhausted {
		escalations = append(escalations, FindingBudgetExhausted)
	}
	if standing.HostUnresponsive {
		escalations = append(escalations, FindingHostUnresponsive)
	}
	evaluation.Outcome = compose(blocks, escalations)
	evaluation.Faults = GroupFaults(standing.Entries, evaluation.Decisions)
	return evaluation
}

// Terminal decides the outcome of a criterion under G3-W.
func Terminal(standing Standing, rules Rules) Outcome { return Evaluate(standing, rules).Outcome }

// DecideEntry decides one entry of a pool: a mutant whose votes could not be
// obtained is unsettled for the reason it has none.
func DecideEntry(subject Subject, universe Universe, entry Entry, rules Rules) Decision {
	decision := Decide(subject, universe, entry.Facts, entry.Votes, rules)
	if decision.Class == ClassUnsettled && len(entry.Votes) == 0 && entry.Unjudged != ReasonNone {
		decision.Reason = entry.Unjudged
	}
	return decision
}

// AllSurvived reports whether every test of the set passed against the mutant;
// an error is neither a kill nor a survival.
func (s Subject) AllSurvived(f Facts) bool { return len(s.Tests) > 0 && f.AllSurvived(s.Tests) }

func noCreditFindings(creditBearing, credited int) []Finding {
	switch {
	case creditBearing == 0:
		return []Finding{FindingNoValidMutant}
	case credited == 0:
		return []Finding{FindingNoKilledValidMutant}
	}
	return nil
}

func sensitiveTests(subject Subject) []Finding {
	for _, test := range subject.Tests {
		if subject.SentinelKillers[test] {
			return []Finding{FindingMutationSensitiveTest}
		}
	}
	return nil
}

func compose(blocks, escalations []Finding) Outcome {
	switch {
	case len(blocks) > 0:
		return Outcome{Result: Block, Findings: sortedUnique(slices.Concat(blocks, escalations))}
	case len(escalations) > 0:
		return Outcome{Result: Escalate, Findings: sortedUnique(escalations)}
	}
	return Outcome{Result: Pass}
}

func sortedUnique(findings []Finding) []Finding {
	slices.Sort(findings)
	return slices.Compact(findings)
}

// Severity ranks a result: the larger, the more severe.
func (r Result) Severity() int {
	switch r {
	case Block:
		return 2
	case Escalate:
		return 1
	}
	return 0
}

// MinWitnessedMutants is the number of witnessed mutants below which a criterion
// gets one further witness draw.
const MinWitnessedMutants = 2

// searchCounted lists the pool mutants the search counts: the targeted ones and
// the owner-attributed ones, whose presence in the pool reads no test.
func searchCounted(pool []Facts, ref spec.Ref) []Facts {
	var counted []Facts
	for _, facts := range pool {
		if facts.Origin == OriginTargeted || facts.OwnerAttributed(ref) {
			counted = append(counted, facts)
		}
	}
	return counted
}

// witnessedSpread counts the witnessed mutants among the counted ones and the
// distinct sites they edit.
func witnessedSpread(universe Universe, ref spec.Ref, pool []Facts) (witnessed, sites int) {
	seen := map[string]bool{}
	for _, facts := range searchCounted(pool, ref) {
		if !universe.Witnessed(facts, ref) {
			continue
		}
		witnessed++
		for _, site := range facts.Sites {
			seen[site] = true
		}
	}
	return witnessed, len(seen)
}

// StopSearching applies the stop rule of the search: stop once the pool holds at
// least `min_valid` witnessed mutants over at least `min_sites` distinct sites.
// It reads no kill vector and no binding, so the generation requests of a
// criterion are the same whatever its tests.
func StopSearching(universe Universe, ref spec.Ref, pool []Facts, rules policy.Policy) bool {
	witnessed, sites := witnessedSpread(universe, ref, pool)
	return witnessed >= rules.MinValid && sites >= rules.MinSites
}

// NeedsFurtherWitnesses reports whether a criterion whose search ended holds
// fewer than MinWitnessedMutants VALID_e candidates, so that one further witness
// writer is asked. A VALID_e candidate is a targeted mutant of the pool that
// fails, reproduced, a block-bearing witness of the criterion and is neither
// owner-attributed nor identical to the reference in compiled form: VALID_e as
// far as it can be told without a kill vector or a binding, so that the request
// is the same whatever the tests.
func NeedsFurtherWitnesses(universe Universe, ref spec.Ref, pool []Facts) bool {
	candidates := 0
	for _, facts := range pool {
		if validECandidate(universe, ref, facts) {
			candidates++
		}
	}
	return candidates < MinWitnessedMutants
}

func validECandidate(universe Universe, ref spec.Ref, facts Facts) bool {
	return facts.Origin == OriginTargeted && facts.Identity != IdentityIdentical && !facts.OwnerAttributed(ref) &&
		len(universe.FailingBlockBearing(facts, ref)) > 0
}

// FaultClass is one fault of a pool (E3): the mutants that edit the same sites
// and fail the same witnesses, or, failing none, differ from the reference on
// the same inputs.
type FaultClass struct {
	Key       string   `json:"key"`
	Sites     []string `json:"sites"`
	Witnesses []string `json:"witnesses,omitempty"`
	Mutants   []string `json:"mutants"`
}

// GroupFaults groups the mutants of a pool that block or escalate by fault: a
// sentinel and an excluded mutant are no fault. Classes come sorted by key and
// their mutants by id.
func GroupFaults(entries []Entry, decisions map[string]Decision) []FaultClass {
	byKey := map[string]*FaultClass{}
	for _, entry := range entries {
		decision := decisions[entry.Facts.ID]
		if decision.Class == ClassSentinel || decision.Class == ClassExcluded {
			continue
		}
		sites := slices.Sorted(slices.Values(entry.Facts.Sites))
		failed := failedWitnesses(entry.Facts)
		key := faultKey(sites, failed, entry.Facts.Differential)
		class, known := byKey[key]
		if !known {
			class = &FaultClass{Key: key, Sites: sites, Witnesses: failed}
			byKey[key] = class
		}
		class.Mutants = append(class.Mutants, entry.Facts.ID)
	}
	classes := make([]FaultClass, 0, len(byKey))
	for _, class := range byKey {
		slices.Sort(class.Mutants)
		classes = append(classes, *class)
	}
	slices.SortFunc(classes, func(a, b FaultClass) int { return strings.Compare(a.Key, b.Key) })
	return classes
}

func failedWitnesses(f Facts) []string {
	var failed []string
	for id, outcome := range f.WitnessOutcomes {
		if outcome == witness.OutcomeFailed {
			failed = append(failed, id)
		}
	}
	slices.Sort(failed)
	return failed
}

// faultKey digests what makes two mutants one fault: the sites they edit and the
// witnesses they fail, or the inputs the differential search found when they
// fail none.
func faultKey(sites, failed []string, differential Differential) string {
	behaviour := any(failed)
	if len(failed) == 0 {
		behaviour = differential.Witnesses
	}
	canonical, err := format.Canonicalize(struct {
		Sites     []string `json:"sites"`
		Behaviour any      `json:"behaviour,omitempty"`
	}{sites, behaviour})
	if err != nil {
		return strings.Join(sites, "\x00")
	}
	return format.SHA256Hex(canonical)
}

// Contradiction is the ground on which execution refutes a judge's answer.
type Contradiction string

// Contradictions of a judge answer by execution.
const (
	ContradictionNone              Contradiction = ""
	ContradictionEquivalentKilled  Contradiction = "equivalent_but_killed"
	ContradictionEquivalentDiffers Contradiction = "equivalent_but_differs"
	ContradictionWitnessRefused    Contradiction = "witness_refused"
)

// Contradicts tells whether execution refutes the vote: an EQUIVALENT answer for
// a mutant a test killed or whose behaviour differs on a found input, or a
// witness that execution does not confirm. Such an answer is never accepted.
func Contradicts(facts Facts, vote Vote) Contradiction {
	switch {
	case vote.Witness == WitnessRefused:
		return ContradictionWitnessRefused
	case vote.Answer != AnswerEquivalent:
		return ContradictionNone
	case facts.KilledByAny():
		return ContradictionEquivalentKilled
	case facts.Differential.Status == DiffersAt:
		return ContradictionEquivalentDiffers
	}
	return ContradictionNone
}
