package promise_test

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/promise"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	verifiesQuote = " [verifies SP-2180~1]"
	verifiesID    = " [verifies SP-2181~1]"
	verifiesPick  = " [verifies SP-2182~1]"

	readmeName      = "README.md"
	descriptionName = "pr-description"
	issueName       = "issue-12"
	readmeText      = "The parser never mutates its input. It returns a copy."
	descriptionText = "Sorting is stable: equal keys keep their order."
	issueText       = "Empty input must return an empty list."
)

func savedSources() map[string]promise.Source {
	return map[string]promise.Source{
		readmeName:      source(readmeName, promise.SourceReadme, readmeText),
		descriptionName: source(descriptionName, promise.SourceDescription, descriptionText),
		issueName:       source(issueName, promise.SourceIssue, issueText),
	}
}

func source(name string, kind promise.SourceKind, text string) promise.Source {
	return promise.Source{Name: name, Kind: kind, Bytes: []byte(text), Digest: format.SHA256Hex([]byte(text))}
}

func TestCheckQuotes(t *testing.T) {
	t.Run("a quote found byte for byte in its saved source is kept with the digest of the source"+verifiesQuote, func(t *testing.T) {
		got := promise.CheckQuotes([]promise.Extracted{{Quote: "never mutates its input", Source: readmeName}}, savedSources())
		if len(got.Kept) != 1 || len(got.Dropped) != 0 {
			t.Fatalf("kept %v dropped %v", got.Kept, got.Dropped)
		}
		kept := got.Kept[0]
		if kept.Kind != promise.SourceReadme || kept.Promise.Source.Digest.SHA256 != format.SHA256Hex([]byte(readmeText)) {
			t.Fatalf("kept %+v", kept)
		}
	})
	t.Run("every other promise is dropped with its own reason"+verifiesQuote, func(t *testing.T) {
		changed := savedSources()
		stale := changed[issueName]
		stale.Digest = format.SHA256Hex([]byte("an older issue body"))
		changed[issueName] = stale
		cases := map[string]struct {
			extracted promise.Extracted
			reason    promise.DropReason
		}{
			"unknown source":  {promise.Extracted{Quote: "never", Source: "notes.txt"}, promise.DropUnknownSource},
			"digest mismatch": {promise.Extracted{Quote: "Empty input", Source: issueName}, promise.DropSourceChanged},
			"quote absent":    {promise.Extracted{Quote: "never writes to disk", Source: readmeName}, promise.DropQuoteAbsent},
			"case differs":    {promise.Extracted{Quote: "SORTING IS STABLE", Source: descriptionName}, promise.DropQuoteAbsent},
			"empty quote":     {promise.Extracted{Quote: "", Source: readmeName}, promise.DropQuoteAbsent},
		}
		for name, c := range cases {
			got := promise.CheckQuotes([]promise.Extracted{c.extracted}, changed)
			if len(got.Kept) != 0 || len(got.Dropped) != 1 || got.Dropped[0].Reason != c.reason {
				t.Errorf("%s: kept %v dropped %v, want reason %s", name, got.Kept, got.Dropped, c.reason)
			}
		}
	})
	t.Run("a quote kept twice from one source is dropped the second time"+verifiesQuote, func(t *testing.T) {
		twice := promise.Extracted{Quote: "equal keys keep their order", Source: descriptionName}
		got := promise.CheckQuotes([]promise.Extracted{twice, twice}, savedSources())
		if len(got.Kept) != 1 || len(got.Dropped) != 1 || got.Dropped[0].Reason != promise.DropDuplicate {
			t.Fatalf("kept %v dropped %v", got.Kept, got.Dropped)
		}
	})
	t.Run("kept and dropped promises follow the extraction order"+verifiesQuote, func(t *testing.T) {
		extracted := []promise.Extracted{
			{Quote: "Empty input", Source: issueName},
			{Quote: "nowhere", Source: readmeName},
			{Quote: "It returns a copy", Source: readmeName},
			{Quote: "nowhere either", Source: issueName},
		}
		got := promise.CheckQuotes(extracted, savedSources())
		if ids := promise.IDs(got.Kept); !slices.Equal(ids, []string{promise.IDOf(issueName, "Empty input"), promise.IDOf(readmeName, "It returns a copy")}) {
			t.Fatalf("kept %v", ids)
		}
		if len(got.Dropped) != 2 || got.Dropped[0].Extracted.Quote != "nowhere" || got.Dropped[1].Extracted.Quote != "nowhere either" {
			t.Fatalf("dropped %v", got.Dropped)
		}
	})
}

func TestIDOf(t *testing.T) {
	t.Run("the id depends on the source and the quote alone"+verifiesID, func(t *testing.T) {
		if promise.IDOf(readmeName, "a quote") != promise.IDOf(readmeName, "a quote") {
			t.Fatal("same promise, different ids")
		}
		if promise.IDOf(readmeName, "a quote") == promise.IDOf(issueName, "a quote") {
			t.Fatal("two sources share an id")
		}
	})
	t.Run("a quote cannot be moved into the source name"+verifiesID, func(t *testing.T) {
		if promise.IDOf("ab", "c") == promise.IDOf("a", "bc") {
			t.Fatal("the boundary between source and quote is lost")
		}
	})
	t.Run("two promises never share an id"+verifiesID, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			a := rapid.StringN(0, 12, -1).Draw(t, "source a")
			b := rapid.StringN(0, 12, -1).Draw(t, "quote a")
			c := rapid.StringN(0, 12, -1).Draw(t, "source b")
			d := rapid.StringN(0, 12, -1).Draw(t, "quote b")
			if (a != c || b != d) && promise.IDOf(a, b) == promise.IDOf(c, d) {
				t.Fatalf("(%q,%q) and (%q,%q) share an id", a, b, c, d)
			}
		})
	})
}

func keptFrom(kind promise.SourceKind, quote string) promise.Kept {
	return promise.Kept{Kind: kind, Promise: format.ScanPromise{ID: promise.IDOf(string(kind), quote), Quote: quote}}
}

func TestChoose(t *testing.T) {
	t.Run("the description and the issues come before the README, then the extraction order"+verifiesPick, func(t *testing.T) {
		kept := []promise.Kept{
			keptFrom(promise.SourceReadme, "r1"),
			keptFrom(promise.SourceIssue, "i1"),
			keptFrom(promise.SourceReadme, "r2"),
			keptFrom(promise.SourceDescription, "d1"),
		}
		got := promise.Choose(kept)
		want := []string{kept[1].Promise.ID, kept[3].Promise.ID, kept[0].Promise.ID}
		if !slices.Equal(promise.IDs(got.Chosen), want) {
			t.Fatalf("chosen %v, want %v", promise.IDs(got.Chosen), want)
		}
		if len(got.Unchecked) != 1 || got.Unchecked[0] != (promise.Unchecked{Promise: kept[2].Promise.ID, Reason: promise.UncheckedOverCap}) {
			t.Fatalf("unchecked %v", got.Unchecked)
		}
	})
	t.Run("a scan chooses at most three promises, and every other is unchecked"+verifiesPick, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			kept := drawKept(t)
			got := promise.Choose(kept)
			if len(got.Chosen) > promise.MaxPromises || len(got.Chosen)+len(got.Unchecked) != len(kept) {
				t.Fatalf("chose %d and left %d of %d", len(got.Chosen), len(got.Unchecked), len(kept))
			}
		})
	})
	t.Run("a README promise is never chosen over a description or issue promise left out"+verifiesPick, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			got := promise.Choose(drawKept(t))
			leftOutChange := slices.ContainsFunc(got.Unchecked, func(u promise.Unchecked) bool { return u.Promise[0] != 'r' })
			chosenReadme := slices.ContainsFunc(got.Chosen, func(k promise.Kept) bool { return k.Kind == promise.SourceReadme })
			if leftOutChange && chosenReadme {
				t.Fatalf("chose %v, left out %v", promise.IDs(got.Chosen), got.Unchecked)
			}
		})
	})
}

// drawKept draws kept promises whose quote starts with the first letter of the
// kind of their source, so that a test can read the kind from an id's quote.
func drawKept(t *rapid.T) []promise.Kept {
	kinds := []promise.SourceKind{promise.SourceReadme, promise.SourceDescription, promise.SourceIssue}
	drawn := rapid.SliceOfN(rapid.SampledFrom(kinds), 0, 8).Draw(t, "kinds")
	kept := make([]promise.Kept, len(drawn))
	for index, kind := range drawn {
		kept[index] = promise.Kept{Kind: kind, Promise: format.ScanPromise{ID: string(kind[0]) + string(rune('0'+index))}}
	}
	return kept
}
