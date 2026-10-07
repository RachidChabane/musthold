package format_test

import (
	"encoding/json"
	"errors"
	"testing"

	"gopkg.in/yaml.v3"
	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/proofs"
)

// thresholdDocument is a configuration file that sets a threshold.
type thresholdDocument struct {
	Tau *format.ThresholdOverride `yaml:"tau"`
}

// decodeThreshold reads the threshold a configuration sets over a uniform one.
func decodeThreshold(t *testing.T, document string, initial float64) (format.Threshold, error) {
	t.Helper()
	var parsed thresholdDocument
	if err := yaml.Unmarshal([]byte(document), &parsed); err != nil {
		return format.Threshold{}, err
	}
	return parsed.Tau.ApplyTo(format.UniformThreshold(initial)), nil
}

func TestThresholdResolution(t *testing.T) {
	endpoint := proofs.Digest("endpoint")
	threshold := format.Threshold{Default: 0.5, BySource: map[format.ConfidenceSource]float64{
		format.SourceHostVotes:                          0.8,
		format.HostVotesSource(2):                       0.95,
		format.SystemOneSource("nimble:9b@" + endpoint): 0.6,
	}}

	cases := []struct {
		name   string
		source format.ConfidenceSource
		want   float64
	}{
		{"an exact source beats its kind", format.HostVotesSource(2), 0.95},
		{"a kind applies to every source of the kind", format.HostVotesSource(3), 0.8},
		{"an exact system one source is keyed on its own", format.SystemOneSource("nimble:9b@" + endpoint), 0.6},
		{"a source of a kind without an entry takes the default", format.SystemOneSource("tev1:4b@" + endpoint), 0.5},
		{"execution without an entry takes the default", format.SourceExecution, 0.5},
		{"a source the format does not know takes the default", "gut-feeling", 0.5},
		{"a claim without a source takes the default", "", 0.5},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-901~1]", func(t *testing.T) {
			if got := threshold.For(c.source); got != c.want {
				t.Fatalf("For(%q) = %v, want %v", c.source, got, c.want)
			}
		})
	}

	t.Run("a kind of source is its source without the detail [verifies SP-901~1]", func(t *testing.T) {
		for source, want := range map[format.ConfidenceSource]format.ConfidenceSource{
			format.SourceExecution:                    format.SourceExecution,
			format.HostVotesSource(4):                 format.SourceHostVotes,
			format.SystemOneSource("m@" + endpoint):   format.SourceSystemOne,
			format.SystemOneSource("a:b@" + endpoint): format.SourceSystemOne,
		} {
			if kind, known := source.Kind(); !known || kind != want {
				t.Errorf("Kind(%q) = %q, %v, want %q", source, kind, known, want)
			}
		}
		if _, known := format.ConfidenceSource("gut-feeling").Kind(); known {
			t.Error("an unknown source has a kind")
		}
	})

	t.Run("a uniform threshold gives the same value for every source [verifies SP-901~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			value := rapid.Float64Range(0, 1).Draw(t, "value")
			source := rapid.SampledFrom([]format.ConfidenceSource{format.SourceExecution, format.HostVotesSource(3), format.SystemOneSource("m@d"), "unknown"}).Draw(t, "source")
			if got := format.UniformThreshold(value).For(source); got != value {
				t.Fatalf("For(%q) = %v, want %v", source, got, value)
			}
		})
	})
}

func TestThresholdSyntax(t *testing.T) {
	t.Run("a bare number is the default for every source [verifies SP-901~1]", func(t *testing.T) {
		got, err := decodeThreshold(t, "tau: 0.9\n", 0.5)
		if err != nil || got.Default != 0.9 || len(got.BySource) != 0 {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("an integer is a number [verifies SP-901~1]", func(t *testing.T) {
		got, err := decodeThreshold(t, "tau: 1\n", 0.5)
		if err != nil || got.Default != 1 {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	t.Run("a mapping sets the default and the sources it names [verifies SP-901~1]", func(t *testing.T) {
		got, err := decodeThreshold(t, "tau:\n  default: 0.7\n  host-votes: 0.8\n  system-one: 0.9\n  host-votes(2): 0.95\n", 0.5)
		if err != nil {
			t.Fatal(err)
		}
		if got.Default != 0.7 || got.For(format.HostVotesSource(3)) != 0.8 || got.For(format.HostVotesSource(2)) != 0.95 || got.For(format.SystemOneSource("m@d")) != 0.9 {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("a mapping without a default keeps the default it was given [verifies SP-901~1]", func(t *testing.T) {
		got, err := decodeThreshold(t, "tau:\n  system-one: 0.9\n", 0.5)
		if err != nil || got.Default != 0.5 || got.For(format.SystemOneSource("m@d")) != 0.9 {
			t.Fatalf("got %+v, %v", got, err)
		}
	})

	invalid := map[string]string{
		"a value above one":             "tau: 1.5\n",
		"a value below zero":            "tau: -0.1\n",
		"an entry above one":            "tau:\n  host-votes: 1.2\n",
		"a key that names no source":    "tau:\n  gut-feeling: 0.5\n",
		"a key of a malformed source":   "tau:\n  host-votes(0): 0.5\n",
		"an entry that is not a number": "tau:\n  host-votes: high\n",
		"a text":                        "tau: high\n",
		"a list":                        "tau: [0.5]\n",
	}
	for name, document := range invalid {
		t.Run(name+" is refused [verifies SP-901~1]", func(t *testing.T) {
			if _, err := decodeThreshold(t, document, 0.5); err == nil {
				t.Fatal("accepted")
			}
		})
	}

	t.Run("a refusal is an ErrThreshold [verifies SP-901~1]", func(t *testing.T) {
		if err := (format.Threshold{Default: 2}).Validate(); !errors.Is(err, format.ErrThreshold) {
			t.Fatalf("err = %v", err)
		}
		if _, err := decodeThreshold(t, "tau: 7\n", 0.5); !errors.Is(err, format.ErrThreshold) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("an override is written back as a bare number or a mapping [verifies SP-901~1]", func(t *testing.T) {
		number := 0.9
		bare, err := yaml.Marshal(thresholdDocument{Tau: &format.ThresholdOverride{Default: &number}})
		if err != nil || string(bare) != "tau: 0.9\n" {
			t.Fatalf("bare = %q, %v", bare, err)
		}
		keyed, err := yaml.Marshal(thresholdDocument{Tau: &format.ThresholdOverride{Default: &number, BySource: map[format.ConfidenceSource]float64{format.SourceSystemOne: 0.6}}})
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeThreshold(t, string(keyed), 0.1)
		if err != nil || got.Default != 0.9 || got.For(format.SystemOneSource("m@d")) != 0.6 {
			t.Fatalf("round trip of %q gave %+v, %v", keyed, got, err)
		}
	})
}

func TestThresholdJSON(t *testing.T) {
	t.Run("a threshold without entries is a bare number, so a policy keeps its digest [verifies SP-901~1]", func(t *testing.T) {
		encoded, err := json.Marshal(format.UniformThreshold(0.7))
		if err != nil || string(encoded) != "0.7" {
			t.Fatalf("encoded = %s, %v", encoded, err)
		}
	})

	t.Run("a threshold survives an encode and a decode [verifies SP-901~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			original := format.Threshold{Default: rapid.Float64Range(0, 1).Draw(t, "default")}
			if rapid.Bool().Draw(t, "keyed") {
				original.BySource = map[format.ConfidenceSource]float64{
					format.SourceHostVotes:    rapid.Float64Range(0, 1).Draw(t, "kind"),
					format.HostVotesSource(2): rapid.Float64Range(0, 1).Draw(t, "exact"),
				}
			}
			encoded, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			var decoded format.Threshold
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("decode %s: %v", encoded, err)
			}
			for _, source := range []format.ConfidenceSource{format.SourceExecution, format.HostVotesSource(2), format.HostVotesSource(3), format.SystemOneSource("m@d")} {
				if decoded.For(source) != original.For(source) {
					t.Fatalf("For(%q) changed from %v to %v over %s", source, original.For(source), decoded.For(source), encoded)
				}
			}
		})
	})
}
