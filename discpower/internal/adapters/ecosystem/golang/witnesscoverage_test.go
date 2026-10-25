package golang_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
)

const (
	verifiesWitnessCoverage = " [verifies SP-2270~1]"
	tallyFile               = "tally.go"
	discountQualname        = "tally.Discount"
	shippingQualname        = "tally.Shipping"
	tallyDiscountSpec       = "A discount of zero percent leaves the total as it is: Discount(total, 0) is total."
	discountWitnessName     = "TestZeroPercentKeepsTheTotal"
	discountWitnessCode     = "func TestZeroPercentKeepsTheTotal(t *testing.T) {\n\tif got := Discount(200, 0); got != 200 {\n\t\tt.Fatalf(\"Discount(200, 0) = %d\", got)\n\t}\n}"
	emptyWitnessName        = "TestNothingIsCalled"
	emptyWitnessCode        = "func TestNothingIsCalled(t *testing.T) {}"
	failingWitnessName      = "TestFortyShipsFree"
	failingWitnessCode      = "func TestFortyShipsFree(t *testing.T) {\n\tif got := Shipping(40); got != 0 {\n\t\tt.Fatalf(\"Shipping(40) = %d\", got)\n\t}\n}"
	// startUpFile is a file of the module whose definition runs when the test
	// binary starts, before any test.
	startUpFile = "rounding.go"
	startUpCode = "package tally\n\nvar roundingUnit = unit()\n\nfunc unit() int {\n\treturn 1\n}\n"
)

var tallyCriterion = spec.Ref{ID: "promise-zero", Revision: 1}

// layOutTallyWitnesses briefs the Go witness host on the tally module, lays
// the witnesses out as one test file of its package and writes the file into
// the workspace.
func layOutTallyWitnesses(t *testing.T, project scanProject, witnesses ...witness.Witness) map[string]ecosystem.BoundTest {
	t.Helper()
	strategy := goStrategy()
	files := os.DirFS(project.workspace.Dir)
	if _, err := strategy.Witnesses().Brief(files, nil, tallyDiscountSpec); err != nil {
		t.Fatal(err)
	}
	laid, err := strategy.Witnesses().Layout(files, witness.Set{Criterion: tallyCriterion, Imports: goImports, Witnesses: witnesses})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project.workspace.Dir, filepath.FromSlash(laid.Path)), laid.Content, 0o644); err != nil {
		t.Fatal(err)
	}
	return laid.Tests
}

func TestGoWitnessCoverage(t *testing.T) {
	project := openScanProject(t)
	if err := os.WriteFile(filepath.Join(project.workspace.Dir, startUpFile), []byte(startUpCode), 0o644); err != nil {
		t.Fatal(err)
	}
	discount := witness.New(tallyCriterion, discountWitnessName, "zero percent", witness.KindExample, discountWitnessCode)
	empty := witness.New(tallyCriterion, emptyWitnessName, "zero percent", witness.KindExample, emptyWitnessCode)
	failing := witness.New(tallyCriterion, failingWitnessName, "ships free", witness.KindExample, failingWitnessCode)
	tests := layOutTallyWitnesses(t, project, discount, empty, failing)
	bound := []ecosystem.BoundTest{tests[discount.ID], tests[empty.ID], tests[failing.ID]}

	recorded, err := goStrategy().WitnessCoverage().Record(context.Background(), project.workspace, project.toolchain, bound)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("a witness that calls one function runs that function alone, keyed by the id its layout gives it"+verifiesWitnessCoverage, func(t *testing.T) {
		want := []footprint.Ref{{File: tallyFile, Qualname: discountQualname}}
		if got := recorded[tests[discount.ID].ID]; !slices.Equal(got, want) {
			t.Fatalf("units = %v, want %v", got, want)
		}
	})
	t.Run("what the binary runs when it starts is no witness's: a witness that calls nothing runs no unit"+verifiesWitnessCoverage, func(t *testing.T) {
		got, known := recorded[tests[empty.ID].ID]
		if !known || len(got) != 0 {
			t.Fatalf("units = %v, known %t", got, known)
		}
	})
	t.Run("a witness the reference fails still runs what it ran"+verifiesWitnessCoverage, func(t *testing.T) {
		want := []footprint.Ref{{File: tallyFile, Qualname: shippingQualname}}
		if got := recorded[tests[failing.ID].ID]; !slices.Equal(got, want) {
			t.Fatalf("units = %v, want %v", got, want)
		}
	})
	t.Run("the entries of the witnesses together are the units either runs"+verifiesWitnessCoverage, func(t *testing.T) {
		ids := []ecosystem.TestID{tests[discount.ID].ID, tests[failing.ID].ID}
		want := []footprint.Ref{{File: tallyFile, Qualname: discountQualname}, {File: tallyFile, Qualname: shippingQualname}}
		if got := recorded.EntriesOf(ids); !slices.Equal(got, want) {
			t.Fatalf("entries = %v, want %v", got, want)
		}
	})
}
