package promise

import (
	"errors"

	"github.com/RachidChabane/musthold/format"
)

// Extracted is a promise as the extraction ticket returned it: a quote and the
// name of the source it claims to be taken from.
type Extracted struct {
	Quote  string
	Source string
}

// Source is a document saved under the session of the scan: its bytes and the
// digest recorded when it was saved.
type Source struct {
	Name   string
	Kind   SourceKind
	Bytes  []byte
	Digest string
}

// Kept is a promise whose quote the check upheld.
type Kept struct {
	Promise format.ScanPromise
	Kind    SourceKind
}

// DropReason says why the quote check dropped a promise.
type DropReason string

// Reasons a promise is dropped.
const (
	// DropUnknownSource: the promise names no saved source.
	DropUnknownSource DropReason = "unknown_source"
	// DropSourceChanged: the saved source no longer has the digest recorded for it.
	DropSourceChanged DropReason = "source_changed"
	// DropQuoteAbsent: the quote is empty or not a byte-exact part of its source.
	DropQuoteAbsent DropReason = "quote_absent"
	// DropDuplicate: the same quote of the same source was already kept.
	DropDuplicate DropReason = "duplicate"
)

// Dropped is an extracted promise the check did not keep.
type Dropped struct {
	Extracted Extracted
	Reason    DropReason
}

// QuoteCheck is the outcome of the quote check, both lists in extraction order.
type QuoteCheck struct {
	Kept    []Kept
	Dropped []Dropped
}

// CheckQuotes keeps the extracted promises whose source was saved and holds
// their quote byte for byte (format.CheckQuote), and reports the others with a
// typed reason. A quote already kept from the same source is dropped as a
// duplicate. The result follows the extraction order and does not depend on
// the order of the map.
func CheckQuotes(extracted []Extracted, saved map[string]Source) QuoteCheck {
	var result QuoteCheck
	seen := map[string]bool{}
	for _, candidate := range extracted {
		kept, reason := checkOne(candidate, saved)
		if reason == "" {
			if seen[kept.Promise.ID] {
				reason = DropDuplicate
			} else {
				seen[kept.Promise.ID] = true
				result.Kept = append(result.Kept, kept)
				continue
			}
		}
		result.Dropped = append(result.Dropped, Dropped{Extracted: candidate, Reason: reason})
	}
	return result
}

func checkOne(candidate Extracted, saved map[string]Source) (Kept, DropReason) {
	source, found := saved[candidate.Source]
	if !found {
		return Kept{}, DropUnknownSource
	}
	scanPromise := format.ScanPromise{
		ID:     IDOf(candidate.Source, candidate.Quote),
		Quote:  candidate.Quote,
		Source: format.ScanSource{Name: source.Name, Digest: format.Digest{SHA256: source.Digest}},
	}
	if err := format.CheckQuote(source.Bytes, scanPromise); err != nil {
		return Kept{}, dropReasonOf(err)
	}
	return Kept{Promise: scanPromise, Kind: source.Kind}, ""
}

func dropReasonOf(err error) DropReason {
	if errors.Is(err, format.ErrQuoteSourceChanged) {
		return DropSourceChanged
	}
	return DropQuoteAbsent
}
