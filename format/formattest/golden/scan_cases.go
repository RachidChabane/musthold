package golden

import (
	"encoding/json"
	"slices"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/proofs"
)

// ScanExpectation is the result every reader of scan statements must give for a
// scan vector.
type ScanExpectation struct {
	// Violations lists the violation codes of the decoded scan statement.
	Violations []format.ViolationCode `json:"violations"`
	// Schema is the result of validating the document against the scan schema.
	Schema string `json:"schema"`
}

// ScanCase is one golden vector of the scan statement.
type ScanCase struct {
	Name     string
	Document []byte
	ScanExpectation
}

// ScanManifestEntry is the manifest line of one scan vector.
type ScanManifestEntry struct {
	File string `json:"file"`
	ScanExpectation
}

// FileName returns the file name of the vector.
func (c ScanCase) FileName() string { return c.Name + ".json" }

// ScanManifest lists the scan vectors in the order of ScanCases.
func ScanManifest(cases []ScanCase) []ScanManifestEntry {
	entries := make([]ScanManifestEntry, len(cases))
	for index, c := range cases {
		entries[index] = ScanManifestEntry{File: c.FileName(), ScanExpectation: c.ScanExpectation}
	}
	return entries
}

func scanValid() ScanExpectation {
	return ScanExpectation{Violations: []format.ViolationCode{}, Schema: SchemaValid}
}

func scanBreaks(schema string, codes ...format.ViolationCode) ScanExpectation {
	return ScanExpectation{Violations: codes, Schema: schema}
}

// scanStatementCase is the valid scan statement among the proof vectors: every
// reader of proofs refuses its predicate type.
func scanStatementCase() Case {
	return Case{
		Name:        "scan-statement-is-not-a-proof",
		Document:    encode(proofs.Scan()),
		Expectation: Expectation{Decode: DecodeUnknownPredicateType, Violations: []format.ViolationCode{}, Schema: SchemaUnknownPredicateType},
	}
}

// ScanCases returns every golden vector of the scan statement.
func ScanCases() []ScanCase {
	type scan = format.ScanStatement
	build := func(name string, expectation ScanExpectation, mutate func(*scan)) ScanCase {
		return ScanCase{Name: name, Document: encode(proofs.Scan(mutate)), ScanExpectation: expectation}
	}
	firstProven := func(s *scan) *format.ScanMutant {
		index := slices.IndexFunc(s.Predicate.Mutants, func(m format.ScanMutant) bool { return m.Proven() })
		return &s.Predicate.Mutants[index]
	}
	testOf := func(s *scan, class format.ScanClass) *format.ScanTest {
		index := slices.IndexFunc(s.Predicate.Tests, func(t format.ScanTest) bool { return t.Class == class })
		return &s.Predicate.Tests[index]
	}
	return []ScanCase{
		{Name: "scan-valid", Document: encode(proofs.Scan()), ScanExpectation: scanValid()},
		build("scan-valid-repository", scanValid(), func(s *scan) {
			s.Predicate.Mode = format.ScanModeRepository
			s.Predicate.Base = ""
		}),
		{Name: "scan-valid-unknown-members", Document: withUnknownMembers(proofs.Scan()), ScanExpectation: scanValid()},
		{Name: "scan-with-requirement", Document: withRequirement(proofs.Scan()), ScanExpectation: scanBreaks(SchemaInvalid, format.CodeRequirement)},
		build("scan-subject-named-after-a-criterion", scanBreaks(SchemaInvalid, format.CodeSubject), func(s *scan) {
			s.Subject[0].Name = format.SubjectName(proofs.RequirementID, proofs.RequirementRevision)
		}),
		build("scan-mutant-id-not-its-edits", scanBreaks(SchemaValid, format.CodeMutantID), func(s *scan) {
			firstProven(s).Edits[0].NewSource += "\n"
		}),
		build("scan-unproven-change-cited", scanBreaks(SchemaValid, format.CodeScanReference), func(s *scan) {
			firstProven(s).Routes = nil
		}),
		build("scan-verifies-nothing-with-a-kill", scanBreaks(SchemaValid, format.CodeScanClass), func(s *scan) {
			test := testOf(s, format.ScanVerifiesNothing)
			test.Changes[0].Outcome = format.ScanKilled
		}),
		build("scan-counts-do-not-add-up", scanBreaks(SchemaValid, format.CodeScanCounts), func(s *scan) {
			s.Predicate.Counts.NotSampled++
		}),
		build("scan-sentinel-proven", scanBreaks(SchemaInvalid, format.CodeSentinelNotEquivalent), func(s *scan) {
			for index := range s.Predicate.Mutants {
				if s.Predicate.Mutants[index].Sentinel {
					s.Predicate.Mutants[index].Routes = []format.ScanRoute{{Kind: format.ScanRouteKilledByTest, Test: proofs.ScanCaughtTest}}
				}
			}
		}),
		build("scan-route-with-foreign-fields", scanBreaks(SchemaInvalid, format.CodeScanRoute), func(s *scan) {
			firstProven(s).Routes[0].Witness = proofs.ScanWitnessID(proofs.ScanWitnessCode)
		}),
		build("scan-broken-promise-on-escalate-only-witness", scanBreaks(SchemaValid, format.CodeScanReference), func(s *scan) {
			for index := range s.Predicate.Witnesses {
				s.Predicate.Witnesses[index].BlockBearing = false
			}
		}),
		build("scan-promises-without-host", scanBreaks(SchemaValid, format.CodeScanReference), func(s *scan) {
			s.Predicate.PromisesChecked = false
		}),
		build("scan-change-mode-without-base", scanBreaks(SchemaInvalid, format.CodeMissingField), func(s *scan) {
			s.Predicate.Base = ""
		}),
		build("scan-unsorted-tests", scanBreaks(SchemaValid, format.CodeUnsorted), func(s *scan) {
			slices.Reverse(s.Predicate.Tests)
		}),
		build("scan-not-sampled-test-listed", scanBreaks(SchemaInvalid, format.CodeUnknownValue), func(s *scan) {
			testOf(s, format.ScanCaught).Class = format.ScanNotSampled
			s.Predicate.Counts.Caught--
			s.Predicate.Counts.NotSampled++
		}),
	}
}

// withRequirement adds a requirement member to a scan's predicate, the shape of
// a gate predicate.
func withRequirement(statement format.ScanStatement) []byte {
	var generic map[string]any
	if err := json.Unmarshal(encode(statement), &generic); err != nil {
		panic(err)
	}
	generic["predicate"].(map[string]any)["requirement"] = map[string]any{
		"id": proofs.RequirementID, "revision": proofs.RequirementRevision, "statementCanonicalization": format.StatementCanonicalizationC1,
	}
	return encode(generic)
}
