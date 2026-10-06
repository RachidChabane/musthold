package projectconfig_test

import (
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/projectconfig"
	"github.com/RachidChabane/spec-proof/format"
)

func TestSettings(t *testing.T) {
	t.Run("new settings disable every proof and record the ecosystems [verifies SP-35~1]", func(t *testing.T) {
		settings := projectconfig.Default([]ecosystem.Name{ecosystem.Pytest})
		if len(settings.Proofs) != len(format.Kinds()) {
			t.Fatalf("proofs = %v", settings.Proofs)
		}
		for _, kind := range format.Kinds() {
			if enabled, present := settings.Proofs[kind]; !present || enabled {
				t.Errorf("%s: present %v enabled %v", kind, present, enabled)
			}
		}
		if len(settings.Enabled()) != 0 || len(settings.Ecosystems) != 1 {
			t.Fatalf("settings = %+v", settings)
		}
		if err := settings.Validate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("enabled proofs come in the stable order of the format [verifies SP-35~1]", func(t *testing.T) {
		settings := projectconfig.Default(nil)
		settings.Proofs[format.KindCompleteness] = true
		settings.Proofs[format.KindAlignment] = true
		enabled := settings.Enabled()
		if len(enabled) != 2 || enabled[0] != format.KindAlignment || enabled[1] != format.KindCompleteness {
			t.Fatalf("enabled = %v", enabled)
		}
	})

	t.Run("an unknown proof or ecosystem is refused [verifies SP-35~1]", func(t *testing.T) {
		unknownProof := projectconfig.Settings{Proofs: map[format.Kind]bool{"mutation": true}}
		unknownEcosystem := projectconfig.Settings{Ecosystems: []ecosystem.Name{"cobol"}}
		for name, settings := range map[string]projectconfig.Settings{"proof": unknownProof, "ecosystem": unknownEcosystem} {
			if err := settings.Validate(); !errors.Is(err, projectconfig.ErrInvalidSettings) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
	})
}
