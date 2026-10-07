package power

import (
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
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
	FindingUndeterminedSurvivor   Finding = "undetermined_survivor"
	FindingNoValidMutant          Finding = "no_valid_mutant"
	FindingNoKilledValidMutant    Finding = "no_killed_valid_mutant"
	FindingMutationSensitiveTest  Finding = "mutation_sensitive_test"
	FindingBudgetExhausted        Finding = "budget_exhausted"
	FindingHostUnresponsive       Finding = "host_unresponsive"
	FindingUnexcludedSurvivor     Finding = "unexcluded_survivor"
	FindingExampleExecutorMissing Finding = "example_executor_unavailable"
)

// Entry is one mutant of a pool with the decision taken for it.
type Entry struct {
	Facts    Facts
	Decision Decision
}

// Standing is everything the terminal outcome of one criterion rests on.
type Standing struct {
	Subject           Subject
	CorrectCodePasses bool
	// ExampleFailed is set when an example of the criterion fails on the
	// reference code.
	ExampleFailed bool
	Entries       []Entry
	// BudgetExhausted and HostUnresponsive are set when a judgment could not be
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

// Terminal decides the outcome of a criterion. BLOCK needs an executed failure or
// a surviving VALID mutant; ESCALATE covers every survivor no decider settled,
// the absence of a killed VALID mutant, a mutation-sensitive test and an
// exhausted budget; PASS needs a killed VALID mutant, every survivor excluded
// and nothing to escalate. A pool that holds a surviving VALID mutant blocks even
// when other findings escalate.
func Terminal(standing Standing) Outcome {
	var blocks, escalations []Finding
	if !standing.CorrectCodePasses {
		blocks = append(blocks, FindingCorrectCodeFails)
	}
	if standing.ExampleFailed {
		blocks = append(blocks, FindingExampleFailed)
	}
	valid, credited := 0, 0
	for _, entry := range standing.Entries {
		if entry.Facts.Origin == OriginSentinel {
			continue
		}
		killed := standing.Subject.Killed(entry.Facts)
		switch {
		case entry.Decision.Carries():
			valid++
			if killed && len(standing.Subject.SpecificKillers(entry.Facts)) > 0 {
				credited++
			}
			if !killed {
				blocks = append(blocks, FindingSurvivingValid)
			}
		case killed:
		case entry.Decision.Class == ClassUndetermined:
			escalations = append(escalations, FindingUndeterminedSurvivor)
		}
	}
	if standing.CorrectCodePasses && !standing.ExampleFailed {
		escalations = append(escalations, noCreditFindings(valid, credited)...)
	}
	escalations = append(escalations, sensitiveTests(standing)...)
	if standing.BudgetExhausted {
		escalations = append(escalations, FindingBudgetExhausted)
	}
	if standing.HostUnresponsive {
		escalations = append(escalations, FindingHostUnresponsive)
	}
	return compose(blocks, escalations)
}

func noCreditFindings(valid, credited int) []Finding {
	switch {
	case valid == 0:
		return []Finding{FindingNoValidMutant}
	case credited == 0:
		return []Finding{FindingNoKilledValidMutant}
	}
	return nil
}

func sensitiveTests(standing Standing) []Finding {
	for _, test := range standing.Subject.Tests {
		if standing.Subject.SentinelKillers[test] {
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

// StopSearching applies the stop rule of ADR 0003: stop when a BLOCK is
// established, or when the pool holds at least `min_valid` VALID mutants on at
// least `min_sites` distinct sites, at least one of them killed.
func StopSearching(standing Standing, rules policy.Policy) bool {
	valid, killed := 0, false
	sites := map[string]bool{}
	for _, entry := range standing.Entries {
		if !entry.Decision.Carries() {
			continue
		}
		valid++
		killed = killed || standing.Subject.Killed(entry.Facts)
		for _, site := range entry.Facts.Sites {
			sites[site] = true
		}
		if !standing.Subject.Killed(entry.Facts) {
			return true
		}
	}
	if !standing.CorrectCodePasses || standing.ExampleFailed {
		return true
	}
	return valid >= rules.MinValid && len(sites) >= rules.MinSites && killed
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
