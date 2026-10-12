package operatorgen

import (
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

const sourceWithRepeatedFunction = "def f0(): pass\ndef f1(): pass\ndef f1(): pass\ndef f2(): pass\n"

func TestAmbiguousDefinitions(t *testing.T) {
	engine := fakeEngine{extension: ".py", functions: []string{"f0", "f1", "f2"}}
	config := operator.Config{KOp: 100, Seed: 7}
	source := []mutation.SourceFile{{Path: "a.py", Content: []byte(sourceWithRepeatedFunction)}}
	result, err := newGenerator(t, engine).Generate(Request{Files: source, Config: config})

	t.Run("a definition several units share is left out and the run goes on [verifies SP-2146~1]", func(t *testing.T) {
		if err != nil {
			t.Fatal(err)
		}
		for _, mutant := range append(append([]operator.Mutant{}, result.Operator...), result.Sentinels...) {
			if mutant.Site.Qualname == "f1" {
				t.Fatalf("mutant %s drawn at f1", mutant.ID)
			}
		}
		if len(result.Operator) != 6 {
			t.Fatalf("%d operator mutants", len(result.Operator))
		}
	})

	t.Run("the units left out are counted once each [verifies SP-2146~1]", func(t *testing.T) {
		if result.Stats.AmbiguousUnits != 1 {
			t.Fatalf("AmbiguousUnits = %d", result.Stats.AmbiguousUnits)
		}
	})

	t.Run("a unit outside the admitted sites is not counted [verifies SP-2146~1]", func(t *testing.T) {
		onlyF0 := func(_ mutation.SourceFile, site operator.Site) bool { return site.Qualname == "f0" }
		admitted, err := newGenerator(t, engine).Generate(Request{Files: source, Config: config, Admit: onlyF0})
		if err != nil || admitted.Stats.AmbiguousUnits != 0 {
			t.Fatalf("AmbiguousUnits = %d, %v", admitted.Stats.AmbiguousUnits, err)
		}
	})

	t.Run("a mutant that fails to splice for another reason still fails the run [verifies SP-2146~1]", func(t *testing.T) {
		_, err := newGenerator(t, ghostEngine{engine}).Generate(Request{Files: source, Config: config})
		if !errors.Is(err, ErrUnsplicable) || !errors.Is(err, mutation.ErrUnknownDefinition) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("a file whose units cannot be located fails the run [verifies SP-2146~1]", func(t *testing.T) {
		registry := NewRegistry()
		if err := registry.Register(engine, failingLocator{}); err != nil {
			t.Fatal(err)
		}
		if _, err := NewGenerator(registry).Generate(Request{Files: source, Config: config}); !errors.Is(err, ErrUnsplicable) {
			t.Fatalf("err = %v", err)
		}
	})
}

// failingLocator cannot read any source.
type failingLocator struct{}

func (failingLocator) Locate(string, []byte) ([]ecosystem.Definition, error) {
	return nil, errors.New("syntax error")
}
