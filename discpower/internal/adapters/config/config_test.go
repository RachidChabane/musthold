package config

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/projectconfig"
	"github.com/RachidChabane/musthold/format"
)

func TestDecode(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		expected projectconfig.Settings
		wantErr  error
		anyErr   bool
	}{
		{
			name: "full document [verifies SP-45~1]",
			yaml: "proofs:\n  alignment: true\n  discriminating-power: false\n  completeness: true\necosystems: [pytest, go]\n",
			expected: projectconfig.Settings{
				Proofs: map[format.Kind]bool{
					format.KindAlignment: true, format.KindDiscriminatingPower: false, format.KindCompleteness: true,
				},
				Ecosystems: []ecosystem.Name{ecosystem.Pytest, ecosystem.GoTesting},
			},
		},
		{
			name:     "empty document [verifies SP-45~1]",
			yaml:     "",
			expected: projectconfig.Settings{Proofs: map[format.Kind]bool{}},
		},
		{
			name: "block sequence of ecosystems [verifies SP-45~1]",
			yaml: "ecosystems:\n  - maven\n  - gradle\n",
			expected: projectconfig.Settings{
				Proofs:     map[format.Kind]bool{},
				Ecosystems: []ecosystem.Name{ecosystem.Maven, ecosystem.Gradle},
			},
		},
		{name: "unknown proof [verifies SP-45~1]", yaml: "proofs:\n  vibes: true\n", wantErr: projectconfig.ErrInvalidSettings},
		{name: "unknown ecosystem [verifies SP-45~1]", yaml: "ecosystems: [cobol]\n", wantErr: projectconfig.ErrInvalidSettings},
		{name: "unknown top-level key [verifies SP-45~1]", yaml: "proofs: {}\nextra: 1\n", anyErr: true},
		{name: "proof with a non-boolean value [verifies SP-45~1]", yaml: "proofs:\n  alignment: maybe\n", anyErr: true},
		{name: "duplicate proof [verifies SP-45~1]", yaml: "proofs:\n  alignment: true\n  alignment: false\n", anyErr: true},
		{name: "malformed yaml [verifies SP-45~1]", yaml: "proofs: [", anyErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Decode([]byte(test.yaml))
			if test.wantErr != nil || test.anyErr {
				if err == nil || (test.wantErr != nil && !errors.Is(err, test.wantErr)) {
					t.Fatalf("err = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if !equalSettings(got, test.expected) {
				t.Fatalf("settings = %+v, want %+v", got, test.expected)
			}
		})
	}
}

func TestEncode(t *testing.T) {
	tests := []struct {
		name     string
		settings projectconfig.Settings
		expected string
		wantErr  bool
	}{
		{
			name:     "default settings in kind order [verifies SP-45~1]",
			settings: projectconfig.Default([]ecosystem.Name{ecosystem.Pytest, ecosystem.GoTesting}),
			expected: "proofs:\n  alignment: false\n  discriminating-power: false\n  completeness: false\necosystems:\n  - pytest\n  - go\n",
		},
		{
			name:     "no ecosystem [verifies SP-45~1]",
			settings: projectconfig.Settings{Proofs: map[format.Kind]bool{format.KindCompleteness: true}},
			expected: "proofs:\n  completeness: true\necosystems: []\n",
		},
		{
			name:     "unknown proof [verifies SP-45~1]",
			settings: projectconfig.Settings{Proofs: map[format.Kind]bool{"vibes": true}},
			wantErr:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Encode(test.settings)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if string(got) != test.expected {
				t.Fatalf("encoded %q, want %q", got, test.expected)
			}
		})
	}
}

func TestEncodeThenDecodeRoundTrips(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		settings := projectconfig.Settings{Proofs: map[format.Kind]bool{}}
		for _, kind := range format.Kinds() {
			if rapid.Bool().Draw(t, "present") {
				settings.Proofs[kind] = rapid.Bool().Draw(t, "enabled")
			}
		}
		settings.Ecosystems = rapid.SliceOf(rapid.SampledFrom(ecosystem.Names())).Draw(t, "ecosystems")

		encoded, err := Encode(settings)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if !equalSettings(decoded, settings) {
			t.Fatalf("round trip changed %+v into %+v", settings, decoded)
		}
		again, err := Encode(decoded)
		if err != nil || string(again) != string(encoded) {
			t.Fatalf("encoding is not deterministic: %q vs %q", encoded, again)
		}
	})
}

func equalSettings(a, b projectconfig.Settings) bool {
	return maps.Equal(a.Proofs, b.Proofs) && slices.Equal(a.Ecosystems, b.Ecosystems)
}
