package golang

import (
	"os"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/musthold/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
)

const (
	repeatedInitSourcePath = "testdata/gates/gates.go"
	repeatedInitName       = "gates/gates.go"
	repeatedInitQualname   = "gates.init"
	allowedQualname        = "gates.Allowed"
)

// Go declares no function or method twice, but a package may hold any number of
// init functions, which share one qualified name.
func TestRepeatedInitFunctions(t *testing.T) {
	content, err := os.ReadFile(repeatedInitSourcePath)
	if err != nil {
		t.Fatal(err)
	}
	registry := operatorgen.NewRegistry()
	if err := registry.Register(New(), goparser.NewLocator()); err != nil {
		t.Fatal(err)
	}
	result, err := operatorgen.NewGenerator(registry).Generate(operatorgen.Request{
		Files:  []mutation.SourceFile{{Path: repeatedInitName, Content: content}},
		Config: operator.Config{KOp: 1000, Seed: operator.LocalSeed},
	})

	t.Run("init functions sharing a name do not fail the run [verifies SP-2146~1]", func(t *testing.T) {
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("no mutant or sentinel is drawn inside an init function [verifies SP-2146~1]", func(t *testing.T) {
		for _, mutant := range append(append([]operator.Mutant{}, result.Operator...), result.Sentinels...) {
			if mutant.Site.Qualname == repeatedInitQualname {
				t.Fatalf("mutant %s drawn at %s", mutant.ID, mutant.Site.Qualname)
			}
		}
	})
	t.Run("the other functions are still mutated and the init functions are counted [verifies SP-2146~1]", func(t *testing.T) {
		drawn := false
		for _, mutant := range result.Operator {
			drawn = drawn || mutant.Site.Qualname == allowedQualname
		}
		if !drawn || result.Stats.AmbiguousUnits != 1 {
			t.Fatalf("drawn at %s = %v, AmbiguousUnits = %d", allowedQualname, drawn, result.Stats.AmbiguousUnits)
		}
	})
}
