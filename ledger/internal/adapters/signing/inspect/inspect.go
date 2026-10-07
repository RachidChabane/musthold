// Package inspect reads the statement out of a signed bundle of either identity:
// a DSSE envelope signed with an Ed25519 key, or a Sigstore bundle. It reads the
// payload and checks no signature.
package inspect

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/adapters/signing/sigstore"
)

// ErrUnknownBundle is returned for a document that is neither of the two shapes.
var ErrUnknownBundle = errors.New("neither a DSSE envelope nor a Sigstore bundle")

// Reader is the PayloadReader of the ledger.
type Reader struct{}

// Payload returns the statement bytes of a bundle.
func (Reader) Payload(bundle []byte) ([]byte, error) {
	var shape struct {
		PayloadType string          `json:"payloadType"`
		MediaType   string          `json:"mediaType"`
		DSSE        json.RawMessage `json:"dsseEnvelope"`
	}
	if err := json.Unmarshal(bundle, &shape); err != nil {
		return nil, fmt.Errorf("bundle: %w", err)
	}
	switch {
	case shape.PayloadType != "":
		envelope, err := format.DecodeEnvelope(bundle)
		if err != nil {
			return nil, err
		}
		return envelope.StatementBytes()
	case shape.MediaType != "" || shape.DSSE != nil:
		return sigstore.Payload(bundle)
	default:
		return nil, ErrUnknownBundle
	}
}
