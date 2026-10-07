// Package sigstore wraps in-toto statements into Sigstore bundles. The bundle
// carries the statement bytes unchanged as the payload of a DSSE envelope signed
// by a keypair. Without options no Fulcio, Rekor or timestamp authority is
// contacted and the verification material is the keypair's public key
// identifier; the keyless signer adds a Fulcio certificate and a Rekor entry.
package sigstore

import (
	"errors"
	"fmt"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/sign"

	"github.com/RachidChabane/spec-proof/format"
)

// PayloadType is the DSSE payload type of an in-toto statement.
const PayloadType = format.DSSEPayloadType

// Errors reported when wrapping or reading a bundle.
var (
	ErrEmptyStatement = errors.New("empty statement")
	ErrNoKeypair      = errors.New("no signing keypair")
	ErrNoEnvelope     = errors.New("bundle carries no DSSE envelope")
)

// Wrap signs the statement bytes as the payload of a DSSE envelope and returns
// the serialized Sigstore bundle.
func Wrap(statement []byte, keypair sign.Keypair) ([]byte, error) {
	return wrap(statement, keypair, sign.BundleOptions{})
}

func wrap(statement []byte, keypair sign.Keypair, options sign.BundleOptions) ([]byte, error) {
	if len(statement) == 0 {
		return nil, ErrEmptyStatement
	}
	if keypair == nil {
		return nil, ErrNoKeypair
	}
	content := &sign.DSSEData{Data: statement, PayloadType: PayloadType}
	protoBundle, err := sign.Bundle(content, keypair, options)
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

// KeypairFactory yields the keypair of one signature. Keyless signing uses a
// fresh ephemeral key per statement, so one leaked key never covers two proofs.
type KeypairFactory func() (sign.Keypair, error)

// Signer signs each statement with a fresh keypair and the given bundle options.
type Signer struct {
	newKeypair KeypairFactory
	options    sign.BundleOptions
}

// NewSigner returns a signer. The options say where the verification material comes
// from: a certificate provider and a transparency log for keyless signing.
func NewSigner(newKeypair KeypairFactory, options sign.BundleOptions) *Signer {
	return &Signer{newKeypair: newKeypair, options: options}
}

// Sign returns the serialized Sigstore bundle of the statement.
func (s *Signer) Sign(statement []byte) ([]byte, error) {
	keypair, err := s.newKeypair()
	if err != nil {
		return nil, fmt.Errorf("new keypair: %w", err)
	}
	return wrap(statement, keypair, s.options)
}
