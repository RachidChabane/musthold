package format

import (
	"bytes"
	"errors"
	"fmt"
)

// Errors of the quote check of a promise.
var (
	// ErrQuoteSourceChanged is returned when the source no longer has the digest
	// the promise recorded.
	ErrQuoteSourceChanged = errors.New("the promise's source has another digest")
	// ErrQuoteAbsent is returned when the quote is not a byte-exact part of its
	// source.
	ErrQuoteAbsent = errors.New("the promise's quote is absent from its source")
)

// CheckQuote requires the promise's quote to occur, byte for byte, in the
// source it names, and the source to be the one whose digest the promise
// recorded. A promise that fails the check is dropped, never shown.
func CheckQuote(source []byte, promise ScanPromise) error {
	if SHA256Hex(source) != promise.Source.Digest.SHA256 {
		return fmt.Errorf("%w: %s", ErrQuoteSourceChanged, promise.Source.Name)
	}
	if promise.Quote == "" || !bytes.Contains(source, []byte(promise.Quote)) {
		return fmt.Errorf("%w: %s", ErrQuoteAbsent, promise.Source.Name)
	}
	return nil
}
