// Package inspect reads the statement out of a signed bundle of either identity:
// a DSSE envelope signed with an Ed25519 key, or a Sigstore bundle. It reads the
// payload and checks no signature.
package inspect

import (
	"errors"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/adapters/signing/sigstore"
)

// ErrUnknownBundle is returned for a document that is neither of the two shapes.
var ErrUnknownBundle = errors.New("neither a DSSE envelope nor a Sigstore bundle")

// Reader is the PayloadReader of the ledger.
type Reader struct{}

// Payload returns the statement bytes of a bundle.
func (Reader) Payload(bundle []byte) ([]byte, error) {
	switch format.ShapeOfBundle(bundle) {
	case format.ShapeEnvelope:
		envelope, err := format.DecodeEnvelope(bundle)
		if err != nil {
			return nil, err
		}
		return envelope.StatementBytes()
	case format.ShapeSigstoreBundle:
		return sigstore.Payload(bundle)
	default:
		return nil, ErrUnknownBundle
	}
}
