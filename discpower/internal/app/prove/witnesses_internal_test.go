package prove

import (
	"context"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/coverage"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	verifiesSP1962 = " [verifies SP-1962~1]"
	verifiesSP1969 = " [verifies SP-1969~2]"

	specDocument      = "When several apply, PAR-3 decides first.\n- PAR-3~1: WHEN an order total reaches 50 THE SYSTEM SHALL waive the shipping fee."
	criterionText     = "WHEN an order total reaches 50 THE SYSTEM SHALL waive the shipping fee."
	interfaceEntry    = "parcel.rates: def shipping_fee(total)"
	helperEntry       = "tests.helpers: def order_of(total)"
	conventionsText   = "Each witness is a pytest test function."
	languageText      = "Python, pytest"
	helperTestFile    = "tests/test_other.py"
	otherRunSeed      = 2
	criterionIDMarker = "PAR-3"
)

// briefed gives the fixture's project the spec text and the brief a witness
// writer is told.
func briefed(f fixture) fixture {
	f.project.labels = prompt.NewLabels([]spec.Ref{f.run.c.Ref})
	f.project.specText = f.project.labels.Relabel(specDocument)
	f.project.brief = ports.WitnessBrief{Language: languageText, Conventions: conventionsText, Interface: []string{interfaceEntry}, Helpers: []string{helperEntry}}
	f.run.c.Statement = criterionText
	return f
}

func witnessRequestOf(t *testing.T, f fixture, draw int) hostproto.Request {
	t.Helper()
	request, err := f.run.witnessRequest(draw)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

// witnessRequestID is the request id of a witness request.
func witnessRequestID(t *testing.T, f fixture, draw int) hostproto.RequestID {
	t.Helper()
	id, err := witnessRequestOf(t, f, draw).ID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestWitnessRequest(t *testing.T) {
	t.Run("a witness request carries the relabelled spec, the interface and the helpers, and names no criterion"+verifiesSP1962, func(t *testing.T) {
		request := witnessRequestOf(t, briefed(newFixture()), firstDraw)
		user := request.Prompt.User
		if request.Kind != hostproto.KindGenerateWitness || request.Round == nil || *request.Round != firstDraw || request.Seed == "" {
			t.Fatalf("request %+v", request)
		}
		for _, shown := range []string{"R1 decides first", interfaceEntry, helperEntry, conventionsText, languageText} {
			if !strings.Contains(user, shown) {
				t.Errorf("the prompt does not show %q", shown)
			}
		}
		if strings.Contains(user, criterionIDMarker) {
			t.Error("the prompt names the criterion by its identifier")
		}
	})

	t.Run("two test trees with equal helpers give the same witness request"+verifiesSP1962, func(t *testing.T) {
		first := briefed(newFixture())
		second := briefed(newFixture())
		second.project.testFileDigests = map[string]string{testFile: format.SHA256Hex([]byte("other tests")), helperTestFile: format.SHA256Hex([]byte("more"))}
		second.project.referenceOutcomes = map[ecosystem.TestID]ecosystem.TestOutcome{testName: ecosystem.TestFailed}
		second.run.c.Tests = append(second.run.c.Tests, ecosystem.NewBoundTest("tests.test_other", "test_more", helperTestFile, 1))
		if witnessRequestID(t, first, firstDraw) != witnessRequestID(t, second, firstDraw) {
			t.Error("the tests changed the witness request")
		}
	})

	t.Run("other helpers give another witness request"+verifiesSP1962, func(t *testing.T) {
		first := briefed(newFixture())
		second := briefed(newFixture())
		second.project.brief.Helpers = nil
		if witnessRequestID(t, first, firstDraw) == witnessRequestID(t, second, firstDraw) {
			t.Error("the helpers left the witness request unchanged")
		}
	})
}

// unwitnessed is an ecosystem strategy that cannot run witnesses.
type unwitnessed struct{ ports.Ecosystem }

func TestWithoutAWitnessHost(t *testing.T) {
	t.Run("an ecosystem that cannot run witnesses asks none and takes the footprint from the tests [verifies SP-1971~1]", func(t *testing.T) {
		f := newFixture()
		f.project.deps.Strategy = unwitnessed{}
		if f.project.witnessHost() != nil {
			t.Fatal("a witness host was found")
		}
		entry := footprint.Ref{File: sourceFile, Qualname: operation}
		f.project.runner.Coverage = coverage.Map{testName: {entry}}
		if err := f.run.firstDraw(context.Background()); err != nil || len(f.run.draws) != 0 {
			t.Fatalf("draws %d, error %v", len(f.run.draws), err)
		}
		entries, err := f.run.entriesOf(context.Background())
		if err != nil || len(entries) != 1 || entries[0] != entry {
			t.Errorf("entries %v, error %v", entries, err)
		}
	})
}
