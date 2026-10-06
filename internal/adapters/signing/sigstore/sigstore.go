// Package sigstore wraps in-toto statements into Sigstore bundles. The bundle
// carries the statement bytes unchanged as the payload of a DSSE envelope signed
// by an injected keypair. No Fulcio, Rekor or timestamp authority is contacted:
// the verification material is the keypair's public key identifier.
package sigstore

import (
	"errors"
	"fmt"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/sign"
)

// PayloadType is the DSSE payload type of an in-toto statement.
const PayloadType = "application/vnd.in-toto+json"

// Errors reported when wrapping or reading a bundle.
var (
	ErrEmptyStatement = errors.New("empty statement")
	ErrNoKeypair      = errors.New("no signing keypair")
	ErrNoEnvelope     = errors.New("bundle carries no DSSE envelope")
)

// Wrap signs the statement bytes as the payload of a DSSE envelope and returns
// the serialized Sigstore bundle.
func Wrap(statement []byte, keypair sign.Keypair) ([]byte, error) {
	if len(statement) == 0 {
		return nil, ErrEmptyStatement
	}
	if keypair == nil {
		return nil, ErrNoKeypair
	}
	content := &sign.DSSEData{Data: statement, PayloadType: PayloadType}
	protoBundle, err := sign.Bundle(content, keypair, sign.BundleOptions{})
	if err != nil {
		return nil, fmt.Errorf("sign statement: %w", err)
	}
	serialized, err := (&bundle.Bundle{Bundle: protoBundle}).MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("serialize bundle: %w", err)
	}
	return serialized, nil
}

// Payload parses a serialized bundle and returns its DSSE envelope payload.
func Payload(bundleJSON []byte) ([]byte, error) {
	parsed := &bundle.Bundle{}
	if err := parsed.UnmarshalJSON(bundleJSON); err != nil {
		return nil, fmt.Errorf("parse bundle: %w", err)
	}
	envelope := parsed.GetDsseEnvelope()
	if envelope == nil {
		return nil, ErrNoEnvelope
	}
	return envelope.GetPayload(), nil
}
