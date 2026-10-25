package golang_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
)

const (
	verifiesWitnessRunner = " [verifies SP-2273~1]"
	brokenWitnessName     = "TestDoesNotBuild"
	brokenWitnessCode     = "func TestDoesNotBuild(t *testing.T) {\n\tif Discount(200) != 200 {\n\t\tt.Fatal(\"no\")\n\t}\n}"
)

// selectionOf selects the bound tests of the witness file the way a run of
// the mutant executor does.
func selectionOf(t *testing.T, project scanProject, tests []ecosystem.BoundTest) ecosystem.Selection {
	t.Helper()
	definitions := map[string][]ecosystem.Definition{}
	for _, test := range tests {
		content, err := os.ReadFile(filepath.Join(project.workspace.Dir, filepath.FromSlash(test.File)))
		if err != nil {
			t.Fatal(err)
		}
		located, err := goparser.NewLocator().Locate(test.File, content)
		if err != nil {
			t.Fatal(err)
		}
		definitions[test.File] = located
	}
	selection, err := goStrategy().Selector().Select(tests, definitions)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

func TestGoWitnessRunner(t *testing.T) {
	ctx := context.Background()
	t.Run("witnesses run with go test alone, each read by the id its layout gives it, with no tool the module declares"+verifiesWitnessRunner, func(t *testing.T) {
		project := openScanProject(t)
		passing := witness.New(tallyCriterion, discountWitnessName, "zero percent", witness.KindExample, discountWitnessCode)
		failing := witness.New(tallyCriterion, failingWitnessName, "ships free", witness.KindExample, failingWitnessCode)
		tests := layOutTallyWitnesses(t, project, passing, failing)
		bound := []ecosystem.BoundTest{tests[passing.ID], tests[failing.ID]}
		run, err := goStrategy().WitnessRunner().Run(ctx, project.workspace, project.toolchain, selectionOf(t, project, bound))
		if err != nil {
			t.Fatal(err)
		}
		if run.CollectionFailed || run.TimedOut || run.Outcomes[tests[passing.ID].ID] != ecosystem.TestPassed || run.Outcomes[tests[failing.ID].ID] != ecosystem.TestFailed {
			t.Fatalf("run = %+v", run)
		}
	})
	t.Run("a witness file that does not compile fails the collection"+verifiesWitnessRunner, func(t *testing.T) {
		project := openScanProject(t)
		broken := witness.New(tallyCriterion, brokenWitnessName, "zero percent", witness.KindExample, brokenWitnessCode)
		tests := layOutTallyWitnesses(t, project, broken)
		run, err := goStrategy().WitnessRunner().Run(ctx, project.workspace, project.toolchain, selectionOf(t, project, []ecosystem.BoundTest{tests[broken.ID]}))
		if err != nil {
			t.Fatal(err)
		}
		if !run.CollectionFailed || run.Outcomes[tests[broken.ID].ID] == ecosystem.TestPassed {
			t.Fatalf("run = %+v", run)
		}
	})
}
