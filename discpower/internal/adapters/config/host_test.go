package config

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/projectconfig"
	"github.com/RachidChabane/spec-proof/format"
)

func TestHostSettings(t *testing.T) {
	t.Run("a document without a host section requires attestation and offers no sampling [verifies SP-227~1]", func(t *testing.T) {
		settings, err := Decode([]byte("proofs: {}\n"))
		if err != nil {
			t.Fatal(err)
		}
		if settings.Host.Sampling || !settings.Host.AttestationRequired() {
			t.Fatalf("host = %+v", settings.Host)
		}
	})

	t.Run("sampling on and the attestation opt-out are read [verifies SP-227~1]", func(t *testing.T) {
		settings, err := Decode([]byte("host:\n  sampling: on\n  require_attestation: false\n"))
		if err != nil {
			t.Fatal(err)
		}
		if !settings.Host.Sampling || settings.Host.AttestationRequired() {
			t.Fatalf("host = %+v", settings.Host)
		}
	})

	t.Run("a mistyped host member is refused [verifies SP-227~1]", func(t *testing.T) {
		for _, document := range []string{"host:\n  sampling: sometimes\n", "host:\n  require_attestation: 7\n", "host:\n  verbose: true\n"} {
			if _, err := Decode([]byte(document)); err == nil {
				t.Errorf("%q was accepted", document)
			}
		}
	})

	t.Run("the sections of the other modules are tolerated and read by nobody here [verifies SP-227~1]", func(t *testing.T) {
		document := "proofs:\n  discriminating-power: true\nalignment:\n  anything: [1, 2]\ncompleteness: {}\nsystemone:\n  endpoint: http://localhost\n"
		if _, err := Decode([]byte(document)); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("host settings that differ from the defaults are encoded and decode back [verifies SP-227~1]", func(t *testing.T) {
		optOut := false
		settings := projectconfig.Settings{
			Proofs: map[format.Kind]bool{},
			Host:   projectconfig.HostSettings{Sampling: true, RequireAttestation: &optOut},
		}
		encoded, err := Encode(settings)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(encoded), "host:\n  sampling: on\n  require_attestation: false\n") {
			t.Fatalf("encoded = %q", encoded)
		}
		decoded, err := Decode(encoded)
		if err != nil || !decoded.Host.Sampling || decoded.Host.AttestationRequired() {
			t.Fatalf("decoded = %+v, %v", decoded.Host, err)
		}
	})

	t.Run("default host settings leave the document as it was [verifies SP-227~1]", func(t *testing.T) {
		encoded, err := Encode(projectconfig.Default(nil))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "host") {
			t.Fatalf("encoded = %q", encoded)
		}
	})
}
