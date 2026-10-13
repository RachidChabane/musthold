// Package promise holds the pure rules of the promise flow of `spec-proof scan`
// (ADR 0014): the quote check of the promises a host extracted, the choice of
// at most three of them, the budget of the calls a scan may spend, the
// admission of witnesses, the one finding a promise can carry, and the labels a
// promise puts on tests. It reads no file, runs nothing and calls no model.
package promise

import "github.com/RachidChabane/spec-proof/format"

// SourceKind says where a promise is quoted from. The set is closed: the
// change's diff is never a quotable source.
type SourceKind string

// Kinds of a quotable source.
const (
	// SourceDescription is the title and body of the change, as `gh` returned them.
	SourceDescription SourceKind = "description"
	// SourceIssue is an issue the change closes.
	SourceIssue SourceKind = "issue"
	// SourceReadme is the README at the head commit.
	SourceReadme SourceKind = "readme"
)

// idDomain separates the id of a promise from every other digest of a scan.
const idDomain = "spec-proof scan promise\x00"

// idSeparator ends the source name in the preimage of an id, so that a quote
// cannot be moved into the name.
const idSeparator = "\x00"

// idHexLength is how many hex digits of the digest an id keeps.
const idHexLength = 16

// IDOf is the id of the promise quoting quote from the source named source: a
// digest of the two, so that the same promise has the same id in every
// invocation of a scan.
func IDOf(source, quote string) string {
	return format.SHA256Hex([]byte(idDomain + source + idSeparator + quote))[:idHexLength]
}

// UncheckedReason says why a promise was kept out of the checks of a scan.
type UncheckedReason string

// Reasons a promise is unchecked.
const (
	// UncheckedOverCap: more promises were kept than a scan checks, and this one
	// ranked below the cap.
	UncheckedOverCap UncheckedReason = "over_cap"
	// UncheckedBudget: the calls left in the budget could not cover its writers
	// and its scope votes.
	UncheckedBudget UncheckedReason = "budget"
)

// Unchecked is a promise the scan did not check, with the reason, so that the
// report can name it.
type Unchecked struct {
	Promise string
	Reason  UncheckedReason
}
