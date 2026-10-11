package config

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/projectconfig"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	verifiesSP1972 = " [verifies SP-1972~1]"

	refactorsDirectory = "tests/refactors"
	admissionKey       = "admission:"
)

func TestAdmissionSettings(t *testing.T) {
	t.Run("a document without an admission section names no refactors"+verifiesSP1972, func(t *testing.T) {
		settings, err := Decode([]byte("proofs: {}\n"))
		if err != nil {
			t.Fatal(err)
		}
		if settings.Admission.RefactorsDir != "" {
			t.Errorf("admission = %+v", settings.Admission)
		}
	})
	t.Run("the refactors directory is read"+verifiesSP1972, func(t *testing.T) {
		settings, err := Decode([]byte("admission:\n  refactors: " + refactorsDirectory + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		if settings.Admission.RefactorsDir != refactorsDirectory {
			t.Errorf("admission = %+v", settings.Admission)
		}
	})
	t.Run("an unknown member under admission is refused"+verifiesSP1972, func(t *testing.T) {
		for _, document := range []string{
			"admission:\n  refactor: " + refactorsDirectory + "\n",
			"admission:\n  refactors: " + refactorsDirectory + "\n  strict: true\n",
		} {
			if _, err := Decode([]byte(document)); err == nil {
				t.Errorf("%q was accepted", document)
			}
		}
	})
	t.Run("a refactors directory that is not a text is refused"+verifiesSP1972, func(t *testing.T) {
		if _, err := Decode([]byte("admission:\n  refactors: [a, b]\n")); err == nil {
			t.Error("a list was accepted")
		}
	})
	t.Run("the directory is written back after the host section"+verifiesSP1972, func(t *testing.T) {
		settings := projectconfig.Settings{
			Proofs:    map[format.Kind]bool{},
			Host:      projectconfig.HostSettings{Sampling: true},
			Admission: projectconfig.AdmissionSettings{RefactorsDir: refactorsDirectory},
		}
		encoded, err := Encode(settings)
		if err != nil {
			t.Fatal(err)
		}
		text := string(encoded)
		hostAt, admissionAt := strings.Index(text, "host:"), strings.Index(text, admissionKey)
		if hostAt < 0 || admissionAt < hostAt {
			t.Fatalf("encoded = %q", text)
		}
		if !strings.HasSuffix(text, admissionKey+"\n  refactors: "+refactorsDirectory+"\n") {
			t.Errorf("encoded = %q", text)
		}
	})
	t.Run("nothing is written when no directory is set"+verifiesSP1972, func(t *testing.T) {
		for _, settings := range []projectconfig.Settings{
			projectconfig.Default([]ecosystem.Name{ecosystem.Pytest}),
			{Proofs: map[format.Kind]bool{}, Host: projectconfig.HostSettings{Sampling: true}},
		} {
			encoded, err := Encode(settings)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "admission") || strings.Contains(string(encoded), "refactors") {
				t.Errorf("encoded = %q", encoded)
			}
		}
	})
	t.Run("the setting is kept when the configuration is written again"+verifiesSP1972, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			directory := rapid.StringMatching(`[a-z][a-z0-9_]{0,8}(/[a-z][a-z0-9_]{0,8}){0,3}`).Draw(t, "directory")
			settings := projectconfig.Settings{
				Proofs:    map[format.Kind]bool{},
				Admission: projectconfig.AdmissionSettings{RefactorsDir: directory},
			}
			encoded, err := Encode(settings)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Admission.RefactorsDir != directory {
				t.Fatalf("round trip changed %q into %q", directory, decoded.Admission.RefactorsDir)
			}
			again, err := Encode(decoded)
			if err != nil || string(again) != string(encoded) {
				t.Fatalf("encoding is not deterministic: %q vs %q", encoded, again)
			}
		})
	})
}
