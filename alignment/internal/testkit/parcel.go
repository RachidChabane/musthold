package testkit

import (
	"path/filepath"
	"runtime"

	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
)

// Names of the fixture projects under `fixtures/`.
const (
	PythonParcel = "parcel-python"
	GoParcel     = "parcel-go"
	// ReportFile is the recorded Shallnot report of a fixture project.
	ReportFile = "shallnot-report.json"
	// RecordedShallnot is the script that stands for the shallnot executable.
	RecordedShallnot = "shallnot-recorded.sh"
)

// FixturesDir is the absolute path of the fixtures directory of the module.
func FixturesDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "fixtures")
}

var (
	// Refs of the criteria of the parcel projects.
	Multiply  = criterion.Ref{ID: "PAR-1", Revision: 1}
	Surcharge = criterion.Ref{ID: "PAR-2", Revision: 1}
	Reject    = criterion.Ref{ID: "PAR-3", Revision: 1}
)

// Statement digests the recorded reports give the criteria.
const (
	MultiplyDigest  = "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f809"
	SurchargeDigest = "9f8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a39281706f5e4d3c2b1a0"
	RejectDigest    = "7c3d1f5e9a0b4c28d6e1f7a3b5c9d2e4f8a6b0c1d3e5f7a9b1c3d5e7f9a1b3c5"
)

// Test identities of the Python parcel project, as `classname::name`.
const (
	PyMultiplies   = "tests.test_pricing::test_line_total_multiplies"
	PySingleUnit   = "tests.test_pricing::test_single_unit_costs_its_price"
	PyZeroRejected = "tests.test_pricing::test_zero_quantity_is_rejected"
	PyRejectedLine = "tests.test_pricing::test_price_with_rejected_quantity"
	PyRemote       = "tests.test_pricing::test_remote_zone_adds_surcharge"
	PyNumber       = "tests.test_pricing::test_shipping_cost_is_a_number"
)

// Test labels of the Python parcel project, as an alignment Statement names them.
const (
	PyMultipliesLabel   = "tests.test_pricing.test_line_total_multiplies"
	PySingleUnitLabel   = "tests.test_pricing.test_single_unit_costs_its_price"
	PyZeroRejectedLabel = "tests.test_pricing.test_zero_quantity_is_rejected"
	PyRejectedLineLabel = "tests.test_pricing.test_price_with_rejected_quantity"
	PyRemoteLabel       = "tests.test_pricing.test_remote_zone_adds_surcharge"
	PyNumberLabel       = "tests.test_pricing.test_shipping_cost_is_a_number"
)

// citationWord is spelled in two parts so that no source line reads as a
// requirement tag.
const citationWord = "verif" + "ies"

const goPackage = "example.com/parcelgo"

// goName spells a Go subtest name the way the JUnit reporter does.
func goName(path, id string) string { return path + "_[" + citationWord + "_" + id + "]" }

var (
	goMultipliesName = goName("TestLineTotal/multiplies_the_unit_price_by_the_quantity", "PAR-1~1")
	goRejectsName    = goName("TestLineTotal/rejects_a_zero_quantity", "PAR-3~1")
	goSurchargeName  = goName("TestShippingCost/adds_the_remote_surcharge", "PAR-2~1")
	goPositiveName   = goName("TestShippingCost/returns_a_positive_cost", "PAR-2~1")

	// Test identities of the Go parcel project, as `classname::name`.
	GoMultiplies = goPackage + "::" + goMultipliesName
	GoRejects    = goPackage + "::" + goRejectsName
	GoSurcharge  = goPackage + "::" + goSurchargeName
	GoPositive   = goPackage + "::" + goPositiveName

	// Test labels of the Go parcel project, as an alignment Statement names them.
	GoMultipliesLabel = goPackage + "." + goMultipliesName
	GoRejectsLabel    = goPackage + "." + goRejectsName
	GoSurchargeLabel  = goPackage + "." + goSurchargeName
	GoPositiveLabel   = goPackage + "." + goPositiveName
)

// PythonPowers returns the discriminating-power Statements of the Python parcel
// project: every test but the two that check nothing of their criterion kills the
// mutants of its criterion.
func PythonPowers() []Power {
	files := func(p Power) Power {
		p.TestFiles, p.CodeFiles = []string{"tests/test_pricing.py"}, []string{"src/parcel/pricing.py"}
		return p
	}
	return []Power{
		files(Power{Ref: Multiply, StatementDigest: MultiplyDigest, Tests: []string{PyMultiplies, PyRejectedLine, PySingleUnit}, Mutants: []Mutant{
			{Digest: "m1", Kind: ValidJudged, Killers: []string{PyMultiplies, PySingleUnit}},
			{Digest: "m2", Kind: ValidExample, Killers: []string{PyMultiplies, PySingleUnit}},
			{Digest: "m3", Kind: ValidJudged, Killers: []string{PyMultiplies}},
			{Digest: "s1", Kind: Sentinel},
		}}),
		files(Power{Ref: Surcharge, StatementDigest: SurchargeDigest, Tests: []string{PyNumber, PyRemote}, Mutants: []Mutant{
			{Digest: "p1", Kind: ValidJudged, Killers: []string{PyRemote}},
			{Digest: "p2", Kind: ValidJudged, Killers: []string{PyRemote}},
			{Digest: "s1", Kind: Sentinel},
		}}),
		files(Power{Ref: Reject, StatementDigest: RejectDigest, Tests: []string{PyZeroRejected}, Mutants: []Mutant{
			{Digest: "q1", Kind: ValidExample, Killers: []string{PyZeroRejected}},
			{Digest: "q2", Kind: ValidJudged, Killers: []string{PyZeroRejected}},
			{Digest: "s1", Kind: Sentinel},
		}}),
	}
}

// GoPowers returns the discriminating-power Statements of the Go parcel project.
func GoPowers() []Power {
	files := func(p Power) Power {
		p.TestFiles, p.CodeFiles = []string{"pricing_test.go"}, []string{"pricing.go"}
		return p
	}
	return []Power{
		files(Power{Ref: Multiply, StatementDigest: MultiplyDigest, Tests: []string{GoMultiplies}, Mutants: []Mutant{
			{Digest: "m1", Kind: ValidJudged, Killers: []string{GoMultiplies}},
			{Digest: "m2", Kind: ValidExample, Killers: []string{GoMultiplies}},
		}}),
		files(Power{Ref: Surcharge, StatementDigest: SurchargeDigest, Tests: []string{GoPositive, GoSurcharge}, Mutants: []Mutant{
			{Digest: "p1", Kind: ValidJudged, Killers: []string{GoSurcharge}},
			{Digest: "p2", Kind: ValidJudged, Killers: []string{GoSurcharge}},
		}}),
		files(Power{Ref: Reject, StatementDigest: RejectDigest, Tests: []string{GoRejects}, Mutants: []Mutant{
			{Digest: "q1", Kind: ValidExample, Killers: []string{GoRejects}},
			{Digest: "q2", Kind: ValidJudged, Killers: []string{GoRejects}},
		}}),
	}
}
