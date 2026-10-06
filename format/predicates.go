package format

import "encoding/json"

// Digest is the digest set of a subject or an input. Only SHA-256 is part of the
// format; other algorithms present on the wire are ignored.
type Digest struct {
	SHA256 string `json:"sha256"`
}

// Subject is one in-toto subject.
type Subject struct {
	Name   string `json:"name"`
	Digest Digest `json:"digest"`
}

// RequirementSubject builds the subject of an alignment or discriminating-power
// statement: the criterion `ID~REV` and its statement digest.
func RequirementSubject(requirementID string, revision int, statementDigest string) Subject {
	return Subject{Name: SubjectName(requirementID, revision), Digest: Digest{SHA256: statementDigest}}
}

// SpecSubject builds the subject of a completeness statement: the spec path and
// the hash of the file.
func SpecSubject(specPath, fileDigest string) Subject {
	return Subject{Name: specPath, Digest: Digest{SHA256: fileDigest}}
}

// Requirement identifies the criterion a statement is about.
type Requirement struct {
	ID                        string `json:"id"`
	Revision                  int    `json:"revision"`
	StatementCanonicalization string `json:"statementCanonicalization"`
}

// Module identifies the module that produced a statement.
type Module struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Annotations carries the file globs a file-set input was expanded from.
type Annotations struct {
	Globs []string `json:"globs,omitempty"`
}

// Input is one sorted resource descriptor that the statement depends on.
type Input struct {
	Role        InputRole    `json:"role"`
	Name        string       `json:"name"`
	Digest      Digest       `json:"digest"`
	Annotations *Annotations `json:"annotations,omitempty"`
}

// Decision records how a judged claim was decided (ADR 0003, A7).
type Decision struct {
	Basis                DecisionBasis `json:"basis"`
	Deciders             []string      `json:"deciders"`
	RawConfidence        float64       `json:"rawConfidence"`
	CalibratedConfidence *float64      `json:"calibratedConfidence,omitempty"`
	Threshold            float64       `json:"threshold"`
	EscalationReason     string        `json:"escalationReason,omitempty"`
}

// OperatorScreen counts operator mutants that were screened rather than listed
// (ADR 0003, A7).
type OperatorScreen struct {
	Flagged     int `json:"flagged"`
	ScreenedOut int `json:"screenedOut"`
}

// Extensions is the open part of a predicate. Readers ignore unknown members.
type Extensions struct {
	Decision       *Decision       `json:"decision,omitempty"`
	OperatorScreen *OperatorScreen `json:"operatorScreen,omitempty"`
	Matrix         json.RawMessage `json:"matrix,omitempty"`
}

// Core is the part shared by the three predicates.
type Core struct {
	Module     Module     `json:"module"`
	Inputs     []Input    `json:"inputs"`
	CacheKey   string     `json:"cacheKey"`
	ProducedBy ProducedBy `json:"producedBy"`
	Extensions Extensions `json:"extensions"`
}

// Judge identifies the model that judged an alignment and its masked prompt.
type Judge struct {
	Model        string `json:"model"`
	PromptDigest string `json:"promptDigest"`
	Blind        bool   `json:"blind"`
}

// Alignment is the predicate of one (test, criterion) alignment judgment.
type Alignment struct {
	Requirement Requirement `json:"requirement"`
	Core
	Test    string         `json:"test"`
	Verdict Claim[Verdict] `json:"verdict"`
	Judge   Judge          `json:"judge"`
}

// MutantExtensions is the open part of one mutant entry (ADR 0003, validity
// mapping and A7).
type MutantExtensions struct {
	Decision     *Decision `json:"decision,omitempty"`
	Collateral   bool      `json:"collateral,omitempty"`
	OffTarget    string    `json:"offTarget,omitempty"`
	Unattributed bool      `json:"unattributed,omitempty"`
}

// TestResult is the executed outcome of one test against one mutant.
type TestResult struct {
	Test    string         `json:"test"`
	Outcome Claim[Outcome] `json:"outcome"`
}

// Mutant is one entry of the mutant pool. The mutant body is never part of the
// proof; only its digest is.
type Mutant struct {
	Digest     string            `json:"digest"`
	Validity   Claim[Validity]   `json:"validity"`
	Sentinel   bool              `json:"sentinel,omitempty"`
	Results    []TestResult      `json:"results"`
	Extensions *MutantExtensions `json:"extensions,omitempty"`
}

// Generator identifies the model that generated targeted mutants.
type Generator struct {
	Model string `json:"model"`
}

// ExampleRun is the executed result of one spec example on the reference code
// (ADR 0003, A6).
type ExampleRun struct {
	Index  int                  `json:"index"`
	Result Claim[ExampleResult] `json:"result"`
}

// DiscriminatingPower is the predicate of one criterion's tests-by-mutants matrix.
type DiscriminatingPower struct {
	Requirement Requirement `json:"requirement"`
	Core
	Tests             []string     `json:"tests"`
	CorrectCodePasses Claim[bool]  `json:"correctCodePasses"`
	Mutants           []Mutant     `json:"mutants,omitempty"`
	Generator         Generator    `json:"generator"`
	Examples          []ExampleRun `json:"examples,omitempty"`
}

// Question is one advisory completeness question.
type Question struct {
	Dimension      string   `json:"dimension"`
	Question       string   `json:"question"`
	RequirementIDs []string `json:"requirementIds,omitempty"`
}

// OutOfScope is one dimension the spec owner declared out of scope.
type OutOfScope struct {
	Dimension string `json:"dimension"`
	Reason    string `json:"reason"`
}

// Completeness is the advisory predicate of one spec. It has no verdict field, so
// the gate cannot block on it.
type Completeness struct {
	Core
	Advisory           bool         `json:"advisory"`
	Questions          []Question   `json:"questions,omitempty"`
	OutOfScopeDeclared []OutOfScope `json:"outOfScopeDeclared,omitempty"`
}
