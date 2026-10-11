package operatorgen

import (
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
)

func TestAdmit(t *testing.T) {
	generator := newGenerator(t, fakeEngine{extension: ".py", functions: []string{"f0", "f1", "f2"}})
	onlyF1 := func(_ mutation.SourceFile, site operator.Site) bool { return site.Qualname == "f1" }

	result, err := generator.Generate(Request{Files: files("a.py"), Config: operator.Config{KOp: 100, Seed: 7}, Admit: onlyF1})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("only the admitted sites give operator mutants [verifies SP-2110~1]", func(t *testing.T) {
		if len(result.Operator) != 3 {
			t.Fatalf("%d operator mutants", len(result.Operator))
		}
		for _, mutant := range result.Operator {
			if mutant.Site.Qualname != "f1" {
				t.Fatalf("mutant at %s", mutant.Site.Qualname)
			}
		}
	})
	t.Run("only the admitted sites give sentinels [verifies SP-2110~1]", func(t *testing.T) {
		if len(result.Sentinels) == 0 {
			t.Fatal("no sentinel")
		}
		for _, mutant := range result.Sentinels {
			if mutant.Site.Qualname != "f1" {
				t.Fatalf("sentinel at %s", mutant.Site.Qualname)
			}
		}
	})
	t.Run("the operator mutants are sampled to the requested count after admission [verifies SP-2110~1]", func(t *testing.T) {
		sampled, err := generator.Generate(Request{Files: files("a.py"), Config: operator.Config{KOp: 2, Seed: 7}, Admit: onlyF1})
		if err != nil || len(sampled.Operator) != 2 || sampled.Stats.Screened != 3 {
			t.Fatalf("%d sampled of %d screened, %v", len(sampled.Operator), sampled.Stats.Screened, err)
		}
	})
}
