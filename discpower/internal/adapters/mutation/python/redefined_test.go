package python

import (
	"os"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

const (
	redefinedSourcePath = "testdata/semver/version.py"
	redefinedName       = "semver/version.py"
	redefinedQualname   = "_comparator"
)

func TestRedefinedDefinition(t *testing.T) {
	content, err := os.ReadFile(redefinedSourcePath)
	if err != nil {
		t.Fatal(err)
	}
	registry := operatorgen.NewRegistry()
	if err := registry.Register(New(), treesitter.NewLocator()); err != nil {
		t.Fatal(err)
	}
	result, err := operatorgen.NewGenerator(registry).Generate(operatorgen.Request{
		Files:  []mutation.SourceFile{{Path: redefinedName, Content: content}},
		Config: operator.Config{KOp: 1000, Seed: operator.LocalSeed},
	})

	t.Run("a definition redefined in its file does not fail the run [verifies SP-2146~1]", func(t *testing.T) {
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("no mutant or sentinel is drawn inside the redefined definition [verifies SP-2146~1]", func(t *testing.T) {
		for _, mutant := range append(append([]operator.Mutant{}, result.Operator...), result.Sentinels...) {
			if mutant.Site.Qualname == redefinedQualname {
				t.Fatalf("mutant %s drawn at %s", mutant.ID, mutant.Site.Qualname)
			}
		}
	})
	t.Run("the other definitions are still mutated [verifies SP-2146~1]", func(t *testing.T) {
		drawn := map[string]bool{}
		for _, mutant := range result.Operator {
			drawn[mutant.Site.Qualname] = true
		}
		for _, qualname := range []string{"clamp", "Version.bump"} {
			if !drawn[qualname] {
				t.Fatalf("no mutant at %s, drawn at %v", qualname, drawn)
			}
		}
	})
	t.Run("the left out definition is counted [verifies SP-2146~1]", func(t *testing.T) {
		if result.Stats.AmbiguousUnits != 1 {
			t.Fatalf("AmbiguousUnits = %d", result.Stats.AmbiguousUnits)
		}
	})
}
