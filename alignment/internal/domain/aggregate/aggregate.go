// Package aggregate reads the votes of an alignment request into a stance:
// the modal criterion over the `align_k` tickets, the modal coverage on it and
// its share, label-blind (ADR 0003, "Alignment").
package aggregate

import (
	"cmp"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
)

// Coverage is how much of a criterion a test's assertions check.
type Coverage string

// Coverage levels, from none to full.
const (
	CoverageNone    Coverage = "NONE"
	CoverageWeak    Coverage = "WEAK"
	CoveragePartial Coverage = "PARTIAL"
	CoverageFull    Coverage = "FULL"
)

// Levels lists the coverage levels in ascending order.
func Levels() []Coverage {
	return []Coverage{CoverageNone, CoverageWeak, CoveragePartial, CoverageFull}
}

// Supporting reports whether the coverage lets a test count as verifying its
// criterion: a FULL or a PARTIAL check.
func (c Coverage) Supporting() bool { return c == CoverageFull || c == CoveragePartial }

// Vote is one answer: the criterion the test verifies, as a reference text or
// masking.NoneAnswer, the coverage and the confidence in [0,1]. A vote without
// Credit comes from an executor below the isolation floor, or one the journal
// marks leaked, exposed or conflicting: it may speak against alignment, never
// for it.
type Vote struct {
	Verifies   string
	Coverage   Coverage
	Confidence float64
	Credit     bool
}

// Demote returns the vote as it counts for the declared criterion: a vote
// without credit that would support it counts as an answer that verifies nothing.
func (v Vote) Demote(declared string) Vote {
	if !v.Credit && v.Verifies == declared && v.Coverage.Supporting() {
		return Vote{Verifies: masking.NoneAnswer, Coverage: CoverageNone, Confidence: v.Confidence}
	}
	return v
}

// Result is the aggregation of the votes of one request.
type Result struct {
	ModalChoice    string   `json:"modalChoice"`
	ModalShare     float64  `json:"modalShare"`
	ModalCoverage  Coverage `json:"modalCoverage"`
	MeanConfidence float64  `json:"meanConfidence"`
	Votes          int      `json:"votes"`
	Missing        int      `json:"missing"`
}

// Aggregate reads the votes among a total number of tickets. The share counts
// the modal votes over the total, so a missing vote counts against agreement.
// The modal criterion has the highest count, ties going to the higher mean
// confidence, then to the lexicographically smaller reference; the modal
// coverage is the commonest among the votes on the modal criterion, ties going to
// the lexicographically smaller level.
func Aggregate(votes []Vote, total int) (Result, bool) {
	if len(votes) == 0 || total < len(votes) {
		return Result{Missing: max(total-len(votes), 0)}, false
	}
	grouped := map[string][]Vote{}
	for _, vote := range votes {
		grouped[vote.Verifies] = append(grouped[vote.Verifies], vote)
	}
	choices := make([]string, 0, len(grouped))
	for choice := range grouped {
		choices = append(choices, choice)
	}
	slices.SortFunc(choices, func(a, b string) int {
		return cmp.Or(
			cmp.Compare(len(grouped[b]), len(grouped[a])),
			cmp.Compare(mean(grouped[b]), mean(grouped[a])),
			strings.Compare(a, b),
		)
	})
	modal := choices[0]
	on := grouped[modal]
	return Result{
		ModalChoice:    modal,
		ModalShare:     float64(len(on)) / float64(total),
		ModalCoverage:  modalCoverage(on),
		MeanConfidence: mean(on),
		Votes:          len(votes),
		Missing:        total - len(votes),
	}, true
}

func mean(votes []Vote) float64 {
	sum := 0.0
	for _, vote := range votes {
		sum += vote.Confidence
	}
	return sum / float64(len(votes))
}

func modalCoverage(votes []Vote) Coverage {
	counts := map[Coverage]int{}
	for _, vote := range votes {
		counts[vote.Coverage]++
	}
	levels := make([]Coverage, 0, len(counts))
	for level := range counts {
		levels = append(levels, level)
	}
	slices.SortFunc(levels, func(a, b Coverage) int {
		return cmp.Or(cmp.Compare(counts[b], counts[a]), strings.Compare(string(a), string(b)))
	})
	return levels[0]
}

// Stance is what the judge's votes say about the declared criterion.
type Stance string

// Stances of the judge.
const (
	// Supports: the modal criterion is the declared one, with a supporting
	// coverage, at or above the minimum modal share.
	Supports Stance = "supports"
	// Rejects: the modal criterion is another one or NONE, or the coverage is WEAK
	// or NONE, at or above the minimum modal share.
	Rejects Stance = "rejects"
	// Indeterminate: no answer reaches the minimum modal share.
	Indeterminate Stance = "indeterminate"
)

// StanceFor reads the result against the declared criterion. The comparison with
// the declared criterion is the last step: nothing before it depends on it.
func (r Result) StanceFor(declared string, modalMin float64) Stance {
	switch {
	case r.Votes == 0 || r.ModalShare < modalMin:
		return Indeterminate
	case r.ModalChoice == declared && r.ModalCoverage.Supporting():
		return Supports
	}
	return Rejects
}

// SingleStance reads one vote, as the tie-break ticket gives it, against the
// declared criterion.
func SingleStance(vote Vote, declared string) Stance {
	if demoted := vote.Demote(declared); demoted.Verifies == declared && demoted.Coverage.Supporting() {
		return Supports
	}
	return Rejects
}
