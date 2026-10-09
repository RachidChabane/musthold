package config

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
)

func TestClassifyWitnessDirectory(t *testing.T) {
	t.Run("host.classify.witness_dir is read and kept when the configuration is written again [verifies SP-1917~1]", func(t *testing.T) {
		settings, err := Decode([]byte("host:\n  classify:\n    witness_dir: /var/log/claude-bodies\n"))
		if err != nil || settings.Host.ClassifyWitnessDir != "/var/log/claude-bodies" || settings.Host.IsDefault() {
			t.Fatalf("host = %+v, %v", settings.Host, err)
		}
		encoded, err := Encode(settings)
		if err != nil || !strings.Contains(string(encoded), "host:\n  classify:\n    witness_dir: /var/log/claude-bodies\n") {
			t.Fatalf("encoded = %q, %v", encoded, err)
		}
		again, err := Decode(encoded)
		if err != nil || again.Host.ClassifyWitnessDir != settings.Host.ClassifyWitnessDir {
			t.Fatalf("decoded = %+v, %v", again.Host, err)
		}
	})

	t.Run("another member of host.classify is refused [verifies SP-1917~1]", func(t *testing.T) {
		if _, err := Decode([]byte("host:\n  classify:\n    witness: /x\n")); err == nil {
			t.Fatal("an unknown member was accepted")
		}
	})

	t.Run("the switch of the classification sits in the systemone section and is tolerated [verifies SP-1917~1]", func(t *testing.T) {
		if _, err := Decode([]byte("proofs:\n  discriminating-power: true\nsystemone:\n  classify: shadow\n")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("an exact classification source keys a threshold of the policy [verifies SP-1917~1]", func(t *testing.T) {
		source, _ := format.SingleVoterClassifySource(5, format.ClassifySchemeRot1, "claude-sonnet-5-5")
		settings, err := Decode([]byte("policy:\n  tau_screen:\n    default: 0.2\n    \"" + source.String() + "\": 0.5\n"))
		if err != nil {
			t.Fatal(err)
		}
		policy := settings.Policy()
		if !policy.TauScreen.HasExact(source.Source()) || policy.TauScreen.For(source.Source()) != 0.5 {
			t.Fatalf("tau_screen = %+v", policy.TauScreen)
		}
	})
}
