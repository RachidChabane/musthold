package format

import (
	"cmp"
	"fmt"
	"strings"
)

// CorroborationKind names the executed evidence a judged claim rests on, one
// kind per row of the claim-power table of ADR 0005.
type CorroborationKind string

// Corroboration kinds.
const (
	// CorroborationExampleFailed: an example of the criterion fails on the mutant.
	CorroborationExampleFailed CorroborationKind = "example_failed"
	// CorroborationOtherExampleFailed: an example of another criterion fails on
	// the mutant and none of the criterion's does.
	CorroborationOtherExampleFailed CorroborationKind = "other_example_failed"
	// CorroborationSpecificKill: a test bound to the criterion killed the mutant
	// and killed no sentinel.
	CorroborationSpecificKill CorroborationKind = "specific_kill"
	// CorroborationWitness: a confirmed witness shows the mutant behaves
	// differently from the reference.
	CorroborationWitness CorroborationKind = "f4_witness"
	// CorroborationBuilds: the mutant compiles.
	CorroborationBuilds CorroborationKind = "builds"
	// CorroborationOtherCriteriaPass: the mutant passes the tests of every other
	// criterion.
	CorroborationOtherCriteriaPass CorroborationKind = "other_criteria_pass"
	// CorroborationNoDiffFound: the differential search found no difference.
	CorroborationNoDiffFound CorroborationKind = "no_diff_found"
	// CorroborationSpecificMatrix: the tests-by-mutants matrix of the test is
	// specific or overbroad, neither contaminated nor pinning off-spec behaviour.
	CorroborationSpecificMatrix CorroborationKind = "specific_matrix"
)

var corroborationKinds = []CorroborationKind{
	CorroborationExampleFailed, CorroborationOtherExampleFailed, CorroborationSpecificKill,
	CorroborationWitness, CorroborationBuilds, CorroborationOtherCriteriaPass,
	CorroborationNoDiffFound, CorroborationSpecificMatrix,
}

// CorroborationKinds lists the closed set of corroboration kinds.
func CorroborationKinds() []CorroborationKind {
	return append([]CorroborationKind(nil), corroborationKinds...)
}

// Corroboration is one piece of executed evidence behind a judged claim: its kind
// and the digest of the evidence record.
type Corroboration struct {
	Kind   CorroborationKind `json:"kind"`
	Digest string            `json:"digest"`
}

func compareCorroborations(a, b Corroboration) int {
	return cmp.Or(strings.Compare(string(a.Kind), string(b.Kind)), strings.Compare(a.Digest, b.Digest))
}

func checkCorroborations(sink *collector, path string, corroborations []Corroboration) {
	for index, corroboration := range corroborations {
		entry := fmt.Sprintf("%s[%d]", path, index)
		checkMember(sink, entry+".kind", corroboration.Kind, corroborationKinds)
		checkDigest(sink, entry+".digest", corroboration.Digest)
	}
	checkAscending(sink, path, corroborations, compareCorroborations)
}

// CarriesPassEffect reports whether a judged validity gives its mutant the power
// to block or to earn pass credit. Only VALID does: the exclusions that
// EQUIVALENT and NOT_VIOLATING grant are a separate, policy-bound effect.
func (v Validity) CarriesPassEffect() bool { return v == ValidityValid }
