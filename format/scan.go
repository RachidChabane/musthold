package format

import "encoding/json"

// PredicateTypeScan identifies the scan predicate: the executed record of one
// `spec-proof scan` of a repository. It is not a proof kind: it carries no
// requirement, the readers of proofs refuse it as one, and the gate never reads
// it (ADR 0014).
const PredicateTypeScan = "https://rachid-chabane.com/attestations/scan/v1"

// RunsPerScanOutcome is how many runs settle one outcome of a scan: a test is
// green when it passes the reference this many times out of as many, and it
// kills or survives a mutant only when every one of this many runs agrees.
const RunsPerScanOutcome = 3

// ScanMode tells what a scan sampled.
type ScanMode string

// Scan modes.
const (
	// ScanModeRepository samples tests of the whole repository by a seed.
	ScanModeRepository ScanMode = "repo"
	// ScanModeChange takes the tests that run the functions a change edits
	// against its base commit.
	ScanModeChange ScanMode = "pr"
)

// ScanClass is where one test of the suite stands after a scan.
type ScanClass string

// Scan classes. The first three are the verdicts of the sampled tests that are
// green; the last two hold the rest of the suite, which a scan does not judge.
const (
	// ScanVerifiesNothing: the test passes every proven change in the lines it
	// ran, three runs out of three, and there are enough of them.
	ScanVerifiesNothing ScanClass = "verifies_nothing"
	// ScanCaught: the test fails, three runs out of three, on at least one proven
	// change in the lines it ran.
	ScanCaught ScanClass = "caught"
	// ScanUnproven: the scan holds too little executed evidence to say either.
	ScanUnproven ScanClass = "unproven"
	// ScanNotSampled: a green test the scan did not sample.
	ScanNotSampled ScanClass = "not_sampled"
	// ScanNotGreen: a test that did not pass the reference three runs out of
	// three; it has no verdict.
	ScanNotGreen ScanClass = "not_green"
)

// ScanUnprovenReason says why a sampled green test is unproven.
type ScanUnprovenReason string

// Reasons of an unproven test.
const (
	// ScanTooFewChanges: the test ran fewer proven changes, or fewer sites, than
	// the claim needs.
	ScanTooFewChanges ScanUnprovenReason = "too_few_changes"
	// ScanInconclusiveOutcome: on a proven change in the lines it ran the test
	// timed out, errored, or its runs disagreed, so it neither passed nor failed
	// it.
	ScanInconclusiveOutcome ScanUnprovenReason = "inconclusive_outcome"
	// ScanKillsSentinel: the test failed on a sentinel, a change that keeps
	// behaviour by construction, so its outcomes do not tell behaviour apart.
	ScanKillsSentinel ScanUnprovenReason = "kills_sentinel"
)

// ScanOutcome is the settled outcome of one test on one mutant.
type ScanOutcome string

// Settled outcomes of a test on a mutant.
const (
	// ScanKilled: the test failed every run.
	ScanKilled ScanOutcome = "killed"
	// ScanSurvived: the test passed every run.
	ScanSurvived ScanOutcome = "survived"
	// ScanInconclusive: a run timed out or errored, a run is missing, or the runs
	// disagree. It counts as neither a kill nor a pass.
	ScanInconclusive ScanOutcome = "inconclusive"
)

// ScanRouteKind names the executed evidence that proves a mutant changes
// behaviour. A mutant without a route is unproven and never counts.
type ScanRouteKind string

// Routes of a proven behaviour change.
const (
	// ScanRouteKilledByTest: a test of the suite that is green and failed on no
	// sentinel fails on the mutant every run.
	ScanRouteKilledByTest ScanRouteKind = "killed_by_test"
	// ScanRouteDifferential: the full-resolution differential search found an
	// input on which the mutant's result differs from the reference's, the
	// difference reproduced, and the reference agreed with itself.
	ScanRouteDifferential ScanRouteKind = "f3_difference"
	// ScanRouteWitness: an admitted witness, written blind to every mutant, fails
	// on the mutant, reproduced.
	ScanRouteWitness ScanRouteKind = "witness_failure"
)

// ScanChannel is what a differential difference was observed on. Logs and the
// standard output are never compared.
type ScanChannel string

// Observed channels of a differential difference.
const (
	ScanChannelReturnValue      ScanChannel = "return_value"
	ScanChannelException        ScanChannel = "exception"
	ScanChannelArgumentMutation ScanChannel = "argument_mutation"
)

// ScanOrigin tells who drew a mutant.
type ScanOrigin string

// Origins of a scan mutant.
const (
	// ScanOriginOperator: drawn from the operator or the sentinel catalogue.
	ScanOriginOperator ScanOrigin = "operator"
	// ScanOriginModel: written by the host's model and stored as its edits.
	ScanOriginModel ScanOrigin = "model"
)

// Explicit target kinds of a scan edit. An edit whose target is omitted replaces
// a whole definition.
const (
	ScanTargetModuleAssignment = "module_assignment"
	ScanTargetModuleAddition   = "module_addition"
)

var (
	scanModes        = []ScanMode{ScanModeRepository, ScanModeChange}
	scanVerdicts     = []ScanClass{ScanVerifiesNothing, ScanCaught, ScanUnproven}
	scanReasons      = []ScanUnprovenReason{ScanTooFewChanges, ScanInconclusiveOutcome, ScanKillsSentinel}
	scanOutcomes     = []ScanOutcome{ScanKilled, ScanSurvived, ScanInconclusive}
	scanRouteKinds   = []ScanRouteKind{ScanRouteKilledByTest, ScanRouteDifferential, ScanRouteWitness}
	scanChannels     = []ScanChannel{ScanChannelReturnValue, ScanChannelException, ScanChannelArgumentMutation}
	scanOrigins      = []ScanOrigin{ScanOriginOperator, ScanOriginModel}
	scanEditTargets  = []string{"", ScanTargetModuleAssignment, ScanTargetModuleAddition}
	scanClassesInAll = []ScanClass{ScanVerifiesNothing, ScanCaught, ScanUnproven, ScanNotSampled, ScanNotGreen}
)

// ScanClasses lists every scan class, the verdicts first.
func ScanClasses() []ScanClass { return append([]ScanClass(nil), scanClassesInAll...) }

// ScanVerdicts lists the classes a sampled green test can be given.
func ScanVerdicts() []ScanClass { return append([]ScanClass(nil), scanVerdicts...) }

// ScanStatement is the in-toto Statement v1 of one scan. Its subject is the
// scanned commit and the digest of its tree, never a requirement.
type ScanStatement struct {
	Type          string    `json:"_type"`
	Subject       []Subject `json:"subject"`
	PredicateType string    `json:"predicateType"`
	Predicate     Scan      `json:"predicate"`
}

// NewScanStatement wraps a scan of a commit, whose tree has the given file-set
// digest, in a Statement v1.
func NewScanStatement(commit, treeDigest string, scan Scan) ScanStatement {
	return ScanStatement{
		Type:          StatementType,
		Subject:       []Subject{{Name: commit, Digest: Digest{SHA256: treeDigest}}},
		PredicateType: PredicateTypeScan,
		Predicate:     scan,
	}
}

// Scan is the predicate of one scan: how it drew, what it ran, and what it
// claims of each sampled test.
type Scan struct {
	Module     Module     `json:"module"`
	ProducedBy ProducedBy `json:"producedBy"`
	Mode       ScanMode   `json:"mode"`
	// Base is the commit a change is compared with; set in change mode only.
	Base string `json:"base,omitempty"`
	// Runner names the test runner the scan detected.
	Runner string `json:"runner"`
	// Seed is the decimal seed of every draw of the scan.
	Seed string `json:"seed"`
	// Partial is set when the budget stopped the scan before every sampled test
	// had a verdict; the counts are those reached.
	Partial bool `json:"partial,omitempty"`
	// PromisesChecked is set when a host extracted and witnessed promises.
	PromisesChecked bool                `json:"promisesChecked"`
	Counts          ScanCounts          `json:"counts"`
	Mutants         []ScanMutant        `json:"mutants"`
	Tests           []ScanTest          `json:"tests"`
	Promises        []ScanPromise       `json:"promises,omitempty"`
	Witnesses       []ScanWitness       `json:"witnesses,omitempty"`
	BrokenPromises  []ScanBrokenPromise `json:"brokenPromises,omitempty"`
	Questions       []ScanQuestion      `json:"questions,omitempty"`
	// Requirement is never set: a scan proves no requirement, and a statement
	// that carries one is refused.
	Requirement json.RawMessage `json:"requirement,omitempty"`
}

// ScanCounts splits every test of the suite into the five classes. The verdict
// of "verifies nothing" is read against Judged, never against Tests.
type ScanCounts struct {
	Tests           int `json:"tests"`
	VerifiesNothing int `json:"verifiesNothing"`
	Caught          int `json:"caught"`
	Unproven        int `json:"unproven"`
	NotSampled      int `json:"notSampled"`
	NotGreen        int `json:"notGreen"`
}

// Judged is the denominator of the scan's claim: the sampled green tests, each
// of which has a verdict.
func (c ScanCounts) Judged() int { return c.VerifiesNothing + c.Caught + c.Unproven }

// Of returns the count of one class.
func (c ScanCounts) Of(class ScanClass) int {
	switch class {
	case ScanVerifiesNothing:
		return c.VerifiesNothing
	case ScanCaught:
		return c.Caught
	case ScanUnproven:
		return c.Unproven
	case ScanNotSampled:
		return c.NotSampled
	case ScanNotGreen:
		return c.NotGreen
	}
	return 0
}

// ScanEdit replaces one unit of one file, in the wire form of a mutant's edits:
// the target is omitted for a definition.
type ScanEdit struct {
	File      string `json:"file"`
	Qualname  string `json:"qualname"`
	Target    string `json:"target,omitempty"`
	NewSource string `json:"new_source"`
}

// ScanLine is one line of a file.
type ScanLine struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// ScanMutant is one mutant the scan ran, with its body: its edits, the lines
// they mutate, and the executed routes that prove it changes behaviour.
type ScanMutant struct {
	// ID is the mutant id: the digest of its edits (ScanEditsDigest).
	ID     string     `json:"id"`
	Origin ScanOrigin `json:"origin"`
	// Family is the operator family of an operator mutant.
	Family   string      `json:"family,omitempty"`
	Sentinel bool        `json:"sentinel,omitempty"`
	Edits    []ScanEdit  `json:"edits"`
	Lines    []ScanLine  `json:"lines"`
	Routes   []ScanRoute `json:"routes,omitempty"`
}

// Proven reports whether executed evidence proves the mutant changes behaviour.
func (m ScanMutant) Proven() bool { return len(m.Routes) > 0 }

// ScanRoute is one piece of executed evidence that a mutant changes behaviour.
// Each kind sets its own fields and no other.
type ScanRoute struct {
	Kind ScanRouteKind `json:"kind"`
	// Test is the killing test of a killed_by_test route.
	Test string `json:"test,omitempty"`
	// Input, Channel, Reference and Mutant are the input of an f3_difference
	// route, what was observed, and the two results as the probe wrote them.
	Input     string      `json:"input,omitempty"`
	Channel   ScanChannel `json:"channel,omitempty"`
	Reference string      `json:"reference,omitempty"`
	Mutant    string      `json:"mutant,omitempty"`
	// Witness is the id of the failing witness of a witness_failure route.
	Witness string `json:"witness,omitempty"`
}

// ScanTest is the verdict on one sampled green test.
type ScanTest struct {
	ID     string             `json:"id"`
	Class  ScanClass          `json:"class"`
	Reason ScanUnprovenReason `json:"reason,omitempty"`
	// Changes are the proven mutants whose mutated lines the test ran, each with
	// the test's settled outcome on it.
	Changes []ScanChange `json:"changes,omitempty"`
	// Promises label the test with the promises its changes may break; a label
	// never makes or unmakes a verdict.
	Promises []string `json:"promises,omitempty"`
}

// ScanChange is a test's settled outcome on one proven mutant.
type ScanChange struct {
	Mutant  string      `json:"mutant"`
	Outcome ScanOutcome `json:"outcome"`
}

// ScanSource names a document a promise is quoted from and its digest.
type ScanSource struct {
	Name   string `json:"name"`
	Digest Digest `json:"digest"`
}

// ScanPromise is one promise a host extracted, with the quote that states it in
// its source.
type ScanPromise struct {
	ID     string     `json:"id"`
	Quote  string     `json:"quote"`
	Source ScanSource `json:"source"`
}

// ScanWitness is one witness written for a promise, blind to every mutant.
type ScanWitness struct {
	ID      string `json:"id"`
	Promise string `json:"promise"`
	Name    string `json:"name"`
	Code    string `json:"code"`
	// Admitted is set when the witness passed the reference every run.
	Admitted bool `json:"admitted,omitempty"`
	// BlockBearing is set when, admitted, both scope judges held its output
	// fixed by the promise's quote alone.
	BlockBearing bool `json:"blockBearing,omitempty"`
}

// ScanBrokenPromise is a mutant that fails a block-bearing witness of a promise
// and passes the whole suite.
type ScanBrokenPromise struct {
	Promise string `json:"promise"`
	Mutant  string `json:"mutant"`
	Witness string `json:"witness"`
}

// ScanQuestion is a witness of a promise that the reference fails: a question
// about the promise, never a finding.
type ScanQuestion struct {
	Promise string `json:"promise"`
	Witness string `json:"witness"`
}
