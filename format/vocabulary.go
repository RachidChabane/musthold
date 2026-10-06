package format

import "strconv"

// Statement and predicate identifiers fixed by ADR 0001.
const (
	// StatementType is the in-toto Statement v1 `_type`.
	StatementType = "https://in-toto.io/Statement/v1"

	// PredicateTypeAlignment identifies the alignment predicate.
	PredicateTypeAlignment = "https://rachid-chabane.com/attestations/alignment/v1"
	// PredicateTypeDiscriminatingPower identifies the discriminating-power predicate.
	PredicateTypeDiscriminatingPower = "https://rachid-chabane.com/attestations/discriminating-power/v1"
	// PredicateTypeCompleteness identifies the completeness predicate.
	PredicateTypeCompleteness = "https://rachid-chabane.com/attestations/completeness/v1"

	// DigestAlgorithm is the only digest algorithm of the format.
	DigestAlgorithm = "sha256"
	// Sha256HexLength is the length of a hex-encoded SHA-256 digest.
	Sha256HexLength = 64

	// StatementCanonicalizationC1 names version c1 of the statement
	// canonicalization performed by Shallnot.
	StatementCanonicalizationC1 = "c1"

	// RevisionSeparator separates a requirement ID from its revision in `ID~REV`.
	RevisionSeparator = "~"
	// MinimumRevision is the first revision of a requirement.
	MinimumRevision = 1

	// MinConfidence and MaxConfidence bound the confidence of a judged claim.
	MinConfidence = 0.0
	MaxConfidence = 1.0
)

// Kind names one proof module and the predicate it produces.
type Kind string

// The three proof kinds.
const (
	KindAlignment           Kind = "alignment"
	KindDiscriminatingPower Kind = "discriminating-power"
	KindCompleteness        Kind = "completeness"
)

var predicateTypes = map[Kind]string{
	KindAlignment:           PredicateTypeAlignment,
	KindDiscriminatingPower: PredicateTypeDiscriminatingPower,
	KindCompleteness:        PredicateTypeCompleteness,
}

// Kinds lists every proof kind in a stable order.
func Kinds() []Kind {
	return []Kind{KindAlignment, KindDiscriminatingPower, KindCompleteness}
}

// PredicateType returns the predicate type URI of the kind, or "" for an
// unknown kind.
func (k Kind) PredicateType() string { return predicateTypes[k] }

// KindOfPredicateType resolves a predicate type URI to its kind.
func KindOfPredicateType(predicateType string) (Kind, bool) {
	for kind, uri := range predicateTypes {
		if uri == predicateType {
			return kind, true
		}
	}
	return "", false
}

// SubjectName builds the `ID~REV` subject name of a requirement.
func SubjectName(requirementID string, revision int) string {
	return requirementID + RevisionSeparator + strconv.Itoa(revision)
}

// Class is the evidence class of a claim.
type Class string

// Evidence classes.
const (
	ClassExecuted Class = "executed"
	ClassJudged   Class = "judged"
)

// Verdict is the judged alignment verdict.
type Verdict string

// Alignment verdicts.
const (
	VerdictAligned      Verdict = "ALIGNED"
	VerdictMisaligned   Verdict = "MISALIGNED"
	VerdictUndetermined Verdict = "UNDETERMINED"
)

// Validity is the judged validity of a mutant.
type Validity string

// Mutant validity values.
const (
	ValidityValid        Validity = "VALID"
	ValidityEquivalent   Validity = "EQUIVALENT"
	ValidityNotViolating Validity = "NOT_VIOLATING"
	ValidityUndetermined Validity = "UNDETERMINED"
)

// Outcome is the executed outcome of one test against one mutant.
type Outcome string

// Mutant outcomes.
const (
	OutcomeKilled   Outcome = "killed"
	OutcomeSurvived Outcome = "survived"
	OutcomeError    Outcome = "error"
)

// ExampleResult is the executed result of one spec example on the reference.
type ExampleResult string

// Example results.
const (
	ExamplePassed ExampleResult = "passed"
	ExampleFailed ExampleResult = "failed"
)

// ProducedBy tells where a statement was produced.
type ProducedBy string

// Producers.
const (
	ProducedByLocal ProducedBy = "local"
	ProducedByCI    ProducedBy = "ci"
)

// InputRole is the role of one input descriptor.
type InputRole string

// Input roles: the seven of ADR 0001 plus the two added by ADR 0003 (A1).
const (
	RoleSpec           InputRole = "spec"
	RoleTestSource     InputRole = "test_source"
	RoleCodeTree       InputRole = "code_tree"
	RoleJUnitResult    InputRole = "junit_result"
	RoleMutantSet      InputRole = "mutant_set"
	RoleModelID        InputRole = "model_id"
	RolePromptTemplate InputRole = "prompt_template"
	RoleDecisionPolicy InputRole = "decision_policy"
	RoleExampleBinding InputRole = "example_binding"
)

// DecisionBasis tells how a judged claim was decided (ADR 0003, A7).
type DecisionBasis string

// Decision bases.
const (
	BasisConstruction DecisionBasis = "construction"
	BasisSpecExample  DecisionBasis = "spec_example"
	BasisJudges       DecisionBasis = "judges"
	BasisSystemOne    DecisionBasis = "system_one"
	BasisExecution    DecisionBasis = "execution"
)

var (
	verdicts       = []Verdict{VerdictAligned, VerdictMisaligned, VerdictUndetermined}
	validities     = []Validity{ValidityValid, ValidityEquivalent, ValidityNotViolating, ValidityUndetermined}
	outcomes       = []Outcome{OutcomeKilled, OutcomeSurvived, OutcomeError}
	exampleResults = []ExampleResult{ExamplePassed, ExampleFailed}
	producers      = []ProducedBy{ProducedByLocal, ProducedByCI}
	inputRoles     = []InputRole{
		RoleSpec, RoleTestSource, RoleCodeTree, RoleJUnitResult, RoleMutantSet,
		RoleModelID, RolePromptTemplate, RoleDecisionPolicy, RoleExampleBinding,
	}
	bases = []DecisionBasis{BasisConstruction, BasisSpecExample, BasisJudges, BasisSystemOne, BasisExecution}
)

// Verdicts lists the closed set of alignment verdicts.
func Verdicts() []Verdict { return append([]Verdict(nil), verdicts...) }

// Validities lists the closed set of mutant validities.
func Validities() []Validity { return append([]Validity(nil), validities...) }

// Outcomes lists the closed set of mutant outcomes.
func Outcomes() []Outcome { return append([]Outcome(nil), outcomes...) }

// InputRoles lists the closed set of input roles.
func InputRoles() []InputRole { return append([]InputRole(nil), inputRoles...) }
