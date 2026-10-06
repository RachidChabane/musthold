// Package golden defines the golden vectors of the proof format: valid statements
// of each predicate and negative statements that each break one rule, with the
// result every reader must give. The vectors are committed under
// format/testdata/golden; this package is their authoritative source.
package golden

import (
	"encoding/json"
	"slices"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/internal/testkit/proofs"
)

// Decode results a vector can expect.
const (
	DecodeOK                   = "ok"
	DecodeUnknownPredicateType = "unknown_predicate_type"
)

// Schema results a vector can expect.
const (
	SchemaValid                = "valid"
	SchemaInvalid              = "invalid"
	SchemaUnknownPredicateType = "unknown_predicate_type"
)

// Expectation is the result every reader must give for a vector.
type Expectation struct {
	// Decode is DecodeOK or DecodeUnknownPredicateType.
	Decode string `json:"decode"`
	// Violations lists the domain violation codes of the decoded statement.
	Violations []format.ViolationCode `json:"violations"`
	// Schema is the result of validating the document against the schemas. Rules
	// that a JSON Schema cannot express (sort order, subject equal to the
	// requirement, cache key) are valid here and violations above.
	Schema string `json:"schema"`
}

// Case is one golden vector.
type Case struct {
	Name     string
	Document []byte
	Expectation
}

// ManifestEntry is the manifest line of one vector.
type ManifestEntry struct {
	File string `json:"file"`
	Expectation
}

// FileName returns the file name of the vector.
func (c Case) FileName() string { return c.Name + ".json" }

// Manifest lists the vectors in the order of Cases.
func Manifest(cases []Case) []ManifestEntry {
	entries := make([]ManifestEntry, len(cases))
	for index, c := range cases {
		entries[index] = ManifestEntry{File: c.FileName(), Expectation: c.Expectation}
	}
	return entries
}

const (
	wrongStatementType  = "https://in-toto.io/Statement/v0.1"
	futurePredicateType = "https://rachid-chabane.com/attestations/future/v1"
	futureMember        = "futureMember"
	futureValue         = "added by a later minor version"
	wrongRequirementID  = "PARK-3"
	absoluteSpecPath    = "/etc/specs/parkfee.md"
	unknownInputRole    = "weird_role"
	outOfRange          = 1.5
	someConfidence      = 0.9
)

func valid() Expectation {
	return Expectation{Decode: DecodeOK, Violations: []format.ViolationCode{}, Schema: SchemaValid}
}

func breaks(schema string, codes ...format.ViolationCode) Expectation {
	return Expectation{Decode: DecodeOK, Violations: codes, Schema: schema}
}

func encode(value any) []byte {
	document, err := format.Canonicalize(value)
	if err != nil {
		panic(err)
	}
	return document
}

func withUnknownMembers(statement any) []byte {
	var generic map[string]any
	if err := json.Unmarshal(encode(statement), &generic); err != nil {
		panic(err)
	}
	generic[futureMember] = futureValue
	generic["predicate"].(map[string]any)[futureMember] = map[string]any{"nested": futureValue}
	return encode(generic)
}

func judgedWithoutConfidence[V comparable](value V) format.Claim[V] {
	return format.Claim[V]{Value: value, Class: format.ClassJudged}
}

func withClass[V comparable](value V, class format.Class) format.Claim[V] {
	return format.Claim[V]{Value: value, Class: class}
}

func confidence() *float64 {
	value := someConfidence
	return &value
}

// Cases returns every golden vector.
func Cases() []Case {
	cases := append(validCases(), alignmentCases()...)
	cases = append(cases, discriminatingPowerCases()...)
	cases = append(cases, completenessCases()...)
	return append(cases, envelopeCases()...)
}

func validCases() []Case {
	return []Case{
		{Name: "alignment-valid", Document: encode(proofs.Alignment()), Expectation: valid()},
		{Name: "alignment-valid-unknown-members", Document: withUnknownMembers(proofs.Alignment()), Expectation: valid()},
		{Name: "discriminating-power-valid", Document: encode(proofs.DiscriminatingPower()), Expectation: valid()},
		{Name: "discriminating-power-no-mutants", Expectation: valid(), Document: encode(proofs.DiscriminatingPower(func(s *format.Statement[format.DiscriminatingPower]) {
			s.Predicate.Mutants = nil
			s.Predicate.Examples = nil
		}))},
		{Name: "completeness-valid", Document: encode(proofs.Completeness()), Expectation: valid()},
	}
}

func alignmentCases() []Case {
	type alignment = format.Statement[format.Alignment]
	build := func(name string, expectation Expectation, mutate func(*alignment)) Case {
		return Case{Name: name, Document: encode(proofs.Alignment(mutate)), Expectation: expectation}
	}
	cacheKeyMismatch := proofs.Alignment()
	cacheKeyMismatch.Predicate.CacheKey = proofs.Digest("another cache key")
	return []Case{
		build("alignment-judged-without-confidence", breaks(SchemaInvalid, format.CodeJudgedWithoutConfidence), func(s *alignment) {
			s.Predicate.Verdict = judgedWithoutConfidence(format.VerdictAligned)
		}),
		build("alignment-executed-verdict", breaks(SchemaInvalid, format.CodeWrongClass), func(s *alignment) {
			s.Predicate.Verdict = withClass(format.VerdictAligned, format.ClassExecuted)
		}),
		build("alignment-confidence-out-of-range", breaks(SchemaInvalid, format.CodeConfidenceRange), func(s *alignment) {
			s.Predicate.Verdict = format.Claim[format.Verdict]{Value: format.VerdictAligned, Class: format.ClassJudged, Confidence: floatPointer(outOfRange)}
		}),
		build("alignment-wrong-subject", breaks(SchemaValid, format.CodeSubject), func(s *alignment) {
			s.Subject[0].Name = format.SubjectName(wrongRequirementID, proofs.RequirementRevision)
		}),
		build("alignment-subject-without-revision", breaks(SchemaInvalid, format.CodeSubject), func(s *alignment) {
			s.Subject[0].Name = proofs.RequirementID
		}),
		build("alignment-unsorted-inputs", breaks(SchemaValid, format.CodeUnsorted), func(s *alignment) {
			slices.Reverse(s.Predicate.Inputs)
		}),
		build("alignment-unknown-input-role", breaks(SchemaInvalid, format.CodeUnknownValue), func(s *alignment) {
			s.Predicate.Inputs = proofs.SortInputs(append(s.Predicate.Inputs, proofs.Input(unknownInputRole, "extra")))
		}),
		build("alignment-not-blind", breaks(SchemaInvalid, format.CodeNotBlind), func(s *alignment) {
			s.Predicate.Judge.Blind = false
		}),
		{Name: "alignment-cache-key-mismatch", Document: encode(cacheKeyMismatch), Expectation: breaks(SchemaValid, format.CodeCacheKeyMismatch)},
	}
}

func discriminatingPowerCases() []Case {
	type power = format.Statement[format.DiscriminatingPower]
	build := func(name string, expectation Expectation, mutate func(*power)) Case {
		return Case{Name: name, Document: encode(proofs.DiscriminatingPower(mutate)), Expectation: expectation}
	}
	return []Case{
		build("discriminating-power-executed-with-confidence", breaks(SchemaInvalid, format.CodeExecutedWithConfidence), func(s *power) {
			s.Predicate.CorrectCodePasses = format.Claim[bool]{Value: true, Class: format.ClassExecuted, Confidence: confidence()}
		}),
		build("discriminating-power-validity-executed", breaks(SchemaInvalid, format.CodeWrongClass), func(s *power) {
			s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)].Validity = withClass(format.ValidityValid, format.ClassExecuted)
		}),
		build("discriminating-power-outcome-judged", breaks(SchemaInvalid, format.CodeWrongClass), func(s *power) {
			s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)].Results[0].Outcome = format.Claim[format.Outcome]{Value: format.OutcomeKilled, Class: format.ClassJudged, Confidence: confidence()}
		}),
		build("discriminating-power-example-judged", breaks(SchemaInvalid, format.CodeWrongClass), func(s *power) {
			s.Predicate.Examples[0].Result = format.Claim[format.ExampleResult]{Value: format.ExamplePassed, Class: format.ClassJudged, Confidence: confidence()}
		}),
		build("discriminating-power-sentinel-not-equivalent", breaks(SchemaInvalid, format.CodeSentinelNotEquivalent), func(s *power) {
			for index := range s.Predicate.Mutants {
				if s.Predicate.Mutants[index].Sentinel {
					s.Predicate.Mutants[index].Validity = judgedClaim(format.ValidityValid)
				}
			}
		}),
		build("discriminating-power-unsorted-tests", breaks(SchemaValid, format.CodeUnsorted), func(s *power) {
			slices.Reverse(s.Predicate.Tests)
		}),
		build("discriminating-power-unsorted-mutants", breaks(SchemaValid, format.CodeUnsorted), func(s *power) {
			slices.Reverse(s.Predicate.Mutants)
		}),
		build("discriminating-power-unsorted-examples", breaks(SchemaValid, format.CodeUnsorted), func(s *power) {
			slices.Reverse(s.Predicate.Examples)
		}),
	}
}

func completenessCases() []Case {
	type completeness = format.Statement[format.Completeness]
	build := func(name string, expectation Expectation, mutate func(*completeness)) Case {
		return Case{Name: name, Document: encode(proofs.Completeness(mutate)), Expectation: expectation}
	}
	return []Case{
		build("completeness-not-advisory", breaks(SchemaInvalid, format.CodeNotAdvisory), func(s *completeness) {
			s.Predicate.Advisory = false
		}),
		build("completeness-unsorted-questions", breaks(SchemaValid, format.CodeUnsorted), func(s *completeness) {
			slices.Reverse(s.Predicate.Questions)
		}),
		build("completeness-absolute-spec-path", breaks(SchemaInvalid, format.CodeSubject), func(s *completeness) {
			s.Subject[0].Name = absoluteSpecPath
		}),
	}
}

func envelopeCases() []Case {
	wrongType := proofs.Alignment(func(s *format.Statement[format.Alignment]) { s.Type = wrongStatementType })
	future := proofs.Alignment(func(s *format.Statement[format.Alignment]) { s.PredicateType = futurePredicateType })
	return []Case{
		{Name: "envelope-wrong-statement-type", Document: encode(wrongType), Expectation: breaks(SchemaInvalid, format.CodeStatementType)},
		{Name: "envelope-unknown-predicate-type", Document: encode(future), Expectation: Expectation{Decode: DecodeUnknownPredicateType, Violations: []format.ViolationCode{}, Schema: SchemaUnknownPredicateType}},
	}
}

func judgedClaim[V comparable](value V) format.Claim[V] {
	return format.Claim[V]{Value: value, Class: format.ClassJudged, Confidence: confidence()}
}

func floatPointer(value float64) *float64 { return &value }
