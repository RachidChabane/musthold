package format_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format"
	"pgregory.net/rapid"
)

const (
	haiku  = "claude-haiku-4-5-20251001"
	sonnet = "claude-sonnet-5-5"
)

func mustSource(t testing.TB, k int, voters ...format.ClassifyVoter) format.ClassifySource {
	t.Helper()
	source, err := format.NewClassifySource(k, format.ClassifySchemeRot1, voters)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestClassifySourceGrammar(t *testing.T) {
	t.Run("a single voter source reads host-classify(5,rot1):model*5 [verifies SP-1900~1]", func(t *testing.T) {
		source := mustSource(t, 5, format.ClassifyVoter{Model: haiku, Calls: 5})
		if got, want := source.String(), "host-classify(5,rot1):"+haiku+"*5"; got != want {
			t.Fatalf("source %q, want %q", got, want)
		}
	})

	t.Run("voters are listed by descending count then identifier whatever order they are given in [verifies SP-1900~1]", func(t *testing.T) {
		source := mustSource(t, 5, format.ClassifyVoter{Model: haiku, Calls: 2}, format.ClassifyVoter{Model: sonnet, Calls: 3})
		if got, want := source.String(), "host-classify(5,rot1):"+sonnet+"*3+"+haiku+"*2"; got != want {
			t.Fatalf("source %q, want %q", got, want)
		}
		tied := mustSource(t, 4, format.ClassifyVoter{Model: sonnet, Calls: 2}, format.ClassifyVoter{Model: haiku, Calls: 2})
		if got, want := tied.String(), "host-classify(4,rot1):"+haiku+"*2+"+sonnet+"*2"; got != want {
			t.Fatalf("source %q, want %q", got, want)
		}
	})

	t.Run("the first voter answers the first calls [verifies SP-1900~1]", func(t *testing.T) {
		source := mustSource(t, 5, format.ClassifyVoter{Model: haiku, Calls: 2}, format.ClassifyVoter{Model: sonnet, Calls: 3})
		var models []string
		for call := range 5 {
			model, ok := source.ModelOfCall(call)
			if !ok {
				t.Fatalf("no voter for call %d", call)
			}
			models = append(models, model)
		}
		if got, want := strings.Join(models, " "), strings.Repeat(sonnet+" ", 3)+haiku+" "+haiku; got != want {
			t.Fatalf("models %q, want %q", got, want)
		}
		if _, ok := source.ModelOfCall(5); ok {
			t.Fatal("a sixth call has a voter")
		}
	})

	refused := map[string]string{
		"counts that do not sum to k":     "host-classify(5,rot1):" + haiku + "*4",
		"an unknown scheme":               "host-classify(5,rot2):" + haiku + "*5",
		"a non-canonical voter order":     "host-classify(5,rot1):" + haiku + "*2+" + sonnet + "*3",
		"a model twice":                   "host-classify(4,rot1):" + haiku + "*2+" + haiku + "*2",
		"no voter":                        "host-classify(5,rot1):",
		"a zero count":                    "host-classify(5,rot1):" + haiku + "*0+" + sonnet + "*5",
		"a leading zero in k":             "host-classify(05,rot1):" + haiku + "*5",
		"a missing scheme":                "host-classify(5):" + haiku + "*5",
		"a missing count":                 "host-classify(5,rot1):" + haiku,
		"another kind":                    "host-votes(5)",
		"a missing model":                 "host-classify(5,rot1):*5",
		"a space in the text":             "host-classify(5,rot1): " + haiku + "*5",
		"a negative k":                    "host-classify(-5,rot1):" + haiku + "*5",
		"a trailing voter separator":      "host-classify(5,rot1):" + haiku + "*5+",
		"an empty text":                   "",
		"the system one prefix":           "system-one:" + haiku + "@abc",
		"a closing parenthesis with no :": "host-classify(5,rot1)" + haiku + "*5",
	}
	for name, text := range refused {
		t.Run("a source with "+name+" is refused [verifies SP-1900~1]", func(t *testing.T) {
			if _, err := format.ParseClassifySource(text); !errors.Is(err, format.ErrClassifySource) {
				t.Fatalf("%q: error %v", text, err)
			}
			if strings.HasPrefix(text, "host-classify") && format.ConfidenceSource(text).IsKnown() {
				t.Fatalf("%q is known", text)
			}
		})
	}

	t.Run("a classification source is a known source of the kind host-classify [verifies SP-1900~1]", func(t *testing.T) {
		source := mustSource(t, 5, format.ClassifyVoter{Model: haiku, Calls: 5}).Source()
		kind, known := source.Kind()
		if !source.IsKnown() || !known || kind != format.SourceHostClassify {
			t.Fatalf("known %v kind %q (%v)", source.IsKnown(), kind, known)
		}
		if !slices.Contains(format.SourceKinds(), format.SourceHostClassify) {
			t.Fatalf("kinds %v", format.SourceKinds())
		}
	})

	t.Run("a threshold keys a classification by its kind or by its exact source [verifies SP-1900~1]", func(t *testing.T) {
		exact := mustSource(t, 5, format.ClassifyVoter{Model: haiku, Calls: 5}).Source()
		other := mustSource(t, 5, format.ClassifyVoter{Model: sonnet, Calls: 5}).Source()
		threshold := format.Threshold{Default: 0.5, BySource: map[format.ConfidenceSource]float64{format.SourceHostClassify: 0.9, exact: 0.97}}
		if err := threshold.Validate(); err != nil {
			t.Fatal(err)
		}
		if threshold.For(exact) != 0.97 || threshold.For(other) != 0.9 || threshold.For(format.HostVotesSource(3)) != 0.5 {
			t.Fatalf("thresholds %v %v %v", threshold.For(exact), threshold.For(other), threshold.For(format.HostVotesSource(3)))
		}
		malformed := format.Threshold{Default: 0.5, BySource: map[format.ConfidenceSource]float64{"host-classify(5,rot1):a*4": 0.9}}
		if err := malformed.Validate(); !errors.Is(err, format.ErrThreshold) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a source reads back what it wrote [verifies SP-1900~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := drawSource(t)
			parsed, err := format.ParseClassifySource(source.String())
			if err != nil {
				t.Fatalf("%q: %v", source.String(), err)
			}
			if parsed.String() != source.String() || parsed.K != source.K || len(parsed.Voters) != len(source.Voters) {
				t.Fatalf("%q parsed as %+v", source.String(), parsed)
			}
			if !source.Source().IsKnown() {
				t.Fatalf("%q is not known", source.String())
			}
		})
	})

	t.Run("every call has exactly one voter and the counts per model are the counts of the source [verifies SP-1900~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			source := drawSource(t)
			counts := map[string]int{}
			for call := range source.K {
				model, ok := source.ModelOfCall(call)
				if !ok {
					t.Fatalf("call %d has no voter", call)
				}
				counts[model]++
			}
			for _, voter := range source.Voters {
				if counts[voter.Model] != voter.Calls {
					t.Fatalf("%s answered %d calls, the source says %d", voter.Model, counts[voter.Model], voter.Calls)
				}
			}
		})
	})
}

// drawSource draws a valid source: a few distinct models whose counts sum to k.
func drawSource(t *rapid.T) format.ClassifySource {
	voters := rapid.IntRange(1, 4).Draw(t, "voters")
	models := rapid.SliceOfNDistinct(rapid.StringMatching(`[a-z][a-z0-9.\[\]-]{0,20}`), voters, voters, func(s string) string { return s }).Draw(t, "models")
	var listed []format.ClassifyVoter
	total := 0
	for _, model := range models {
		calls := rapid.IntRange(1, 6).Draw(t, "calls")
		listed = append(listed, format.ClassifyVoter{Model: model, Calls: calls})
		total += calls
	}
	source, err := format.NewClassifySource(total, format.ClassifySchemeRot1, listed)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestClassifyMapping(t *testing.T) {
	digest := format.SHA256Hex([]byte("table"))

	t.Run("the mapping is host-classify.share-isotonic@ then the table digest, and reads back [verifies SP-1901~1]", func(t *testing.T) {
		mapping := format.HostClassifyMapping(digest)
		if mapping != "host-classify.share-isotonic@"+digest {
			t.Fatalf("mapping %q", mapping)
		}
		got, err := format.ParseHostClassifyMapping(mapping)
		if err != nil || got != digest {
			t.Fatalf("digest %q, %v", got, err)
		}
	})

	t.Run("a mapping without a table digest is refused [verifies SP-1901~1]", func(t *testing.T) {
		for _, text := range []string{"", "host-classify.share-isotonic", "host-classify.share-isotonic@", "host-classify.share-isotonic@abc", "host-votes.modal-share", digest} {
			if _, err := format.ParseHostClassifyMapping(text); !errors.Is(err, format.ErrClassifyMapping) {
				t.Errorf("%q: error %v", text, err)
			}
		}
	})
}

func TestClassifyChannelIsInTheClosedSets(t *testing.T) {
	t.Run("the channel host-classify is one of the channels and its profile validates [verifies SP-1903~1]", func(t *testing.T) {
		if !slices.Contains(format.Channels(), format.ChannelHostClassify) {
			t.Fatalf("channels %v", format.Channels())
		}
		profile := format.ExecutorProfile{
			Channel: format.ChannelHostClassify, Harness: format.Harness{Name: "claude-code", Version: "2.1.293"},
			Isolation: format.IsolationRestrictedUnverified, Model: format.ExecutorModel{Name: sonnet, Source: format.ModelSourceDeclared},
		}
		if err := profile.Validate(); err != nil {
			t.Fatal(err)
		}
	})
}
