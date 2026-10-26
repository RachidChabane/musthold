package format_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/formattest/proofs"
)

// violationCodes returns the distinct violation codes carried by err, or nil
// when err is nil.
func violationCodes(t *testing.T, err error) []format.ViolationCode {
	t.Helper()
	if err == nil {
		return nil
	}
	var validation *format.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error %v is not a validation error", err)
	}
	return validation.Codes()
}

func assertCodes(t *testing.T, err error, want ...format.ViolationCode) {
	t.Helper()
	got := violationCodes(t, err)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("violation codes = %v, want %v (error: %v)", got, want, err)
	}
}

func floatPointer(value float64) *float64 { return &value }

// corroborationFixture is a valid piece of executed evidence.
func corroborationFixture() format.Corroboration {
	return format.Corroboration{Kind: format.CorroborationSpecificKill, Digest: proofs.Digest("evidence/specific-kill")}
}

// regularMutant returns the first mutant that is not a sentinel, with a decision
// attached so a test can edit it.
func regularMutant(s *format.Statement[format.DiscriminatingPower]) *format.Mutant {
	mutant := &s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)]
	mutant.Extensions = &format.MutantExtensions{Decision: &format.Decision{
		Basis:            format.BasisJudges,
		Deciders:         []string{"judge-a", "judge-b"},
		ConfidenceSource: format.HostVotesSource(2),
		RawConfidence:    0.9,
		Threshold:        0.7,
	}}
	return mutant
}
