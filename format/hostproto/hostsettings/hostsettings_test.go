package hostsettings_test

import (
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto/hostsettings"
)

func TestParse(t *testing.T) {
	t.Run("a file without a host section requires attestation and offers no sampling [verifies SP-719~1]", func(t *testing.T) {
		for _, config := range []string{"", "proofs: {}\n"} {
			settings, err := hostsettings.Parse([]byte(config))
			if err != nil {
				t.Fatal(err)
			}
			if settings.Sampling || !settings.AttestationRequired() || !settings.IsDefault() {
				t.Fatalf("%q: settings = %+v", config, settings)
			}
		}
	})

	t.Run("sampling on and the attestation opt-out are read [verifies SP-719~1]", func(t *testing.T) {
		settings, err := hostsettings.Parse([]byte("host:\n  sampling: on\n  require_attestation: false\n"))
		if err != nil {
			t.Fatal(err)
		}
		if !settings.Sampling || settings.AttestationRequired() || settings.IsDefault() {
			t.Fatalf("settings = %+v", settings)
		}
	})

	t.Run("a mistyped or unknown host member is refused [verifies SP-719~1]", func(t *testing.T) {
		for _, config := range []string{"host:\n  sampling: sometimes\n", "host:\n  require_attestation: 7\n", "host:\n  verbose: true\n"} {
			if _, err := hostsettings.Parse([]byte(config)); !errors.Is(err, hostsettings.ErrInvalidSection) {
				t.Errorf("%q: error = %v", config, err)
			}
		}
	})

	t.Run("the sections of the modules are tolerated and read by nobody here [verifies SP-719~1]", func(t *testing.T) {
		config := "proofs:\n  alignment: true\nalignment:\n  anything: [1, 2]\ncompleteness: {}\nsystemone:\n  endpoint: http://localhost\nhost:\n  sampling: on\n"
		settings, err := hostsettings.Parse([]byte(config))
		if err != nil || !settings.Sampling {
			t.Fatalf("settings = %+v, %v", settings, err)
		}
	})

	t.Run("a file that is not YAML is refused [verifies SP-719~1]", func(t *testing.T) {
		if _, err := hostsettings.Parse([]byte("host: [unclosed")); !errors.Is(err, hostsettings.ErrInvalidSection) {
			t.Fatalf("error = %v", err)
		}
	})
}
