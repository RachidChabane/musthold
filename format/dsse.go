package format

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// DSSEPayloadType is the payload type of a DSSE envelope that carries an in-toto
// statement.
const DSSEPayloadType = "application/vnd.in-toto+json"

// paeHeader opens the pre-authentication encoding of the DSSE specification.
const paeHeader = "DSSEv1"

// Errors reported when an envelope is read.
var (
	ErrEnvelopeShape = errors.New("malformed DSSE envelope")
)

// PAE returns the DSSE pre-authentication encoding of a payload: the exact bytes
// a DSSE signature covers.
func PAE(payloadType string, payload []byte) []byte {
	encoded := make([]byte, 0, len(paeHeader)+len(payloadType)+len(payload)+paeOverhead)
	encoded = append(encoded, paeHeader...)
	encoded = append(encoded, ' ')
	encoded = strconv.AppendInt(encoded, int64(len(payloadType)), decimalBase)
	encoded = append(encoded, ' ')
	encoded = append(encoded, payloadType...)
	encoded = append(encoded, ' ')
	encoded = strconv.AppendInt(encoded, int64(len(payload)), decimalBase)
	encoded = append(encoded, ' ')
	return append(encoded, payload...)
}

const (
	// paeOverhead is room for the separators and the two decimal lengths.
	paeOverhead = 48
	decimalBase = 10
)

// EnvelopeSignature is one signature of a DSSE envelope. The key identifier is
// the hex SHA-256 of the raw Ed25519 public key.
type EnvelopeSignature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

// Envelope is a DSSE envelope signed with an Ed25519 identity: the exact statement
// bytes, base64-encoded, and the signatures over their pre-authentication
// encoding. Keyless identities travel in Sigstore bundles instead.
type Envelope struct {
	PayloadType string              `json:"payloadType"`
	Payload     string              `json:"payload"`
	Signatures  []EnvelopeSignature `json:"signatures"`
}

// Ed25519KeyID identifies an Ed25519 public key: the hex SHA-256 of its raw bytes.
func Ed25519KeyID(publicKey ed25519.PublicKey) string {
	return SHA256Hex(publicKey)
}

// SignEnvelope wraps statement bytes in a DSSE envelope signed with privateKey.
func SignEnvelope(statement []byte, privateKey ed25519.PrivateKey) Envelope {
	signature := ed25519.Sign(privateKey, PAE(DSSEPayloadType, statement))
	publicKey, _ := privateKey.Public().(ed25519.PublicKey)
	return Envelope{
		PayloadType: DSSEPayloadType,
		Payload:     base64.StdEncoding.EncodeToString(statement),
		Signatures: []EnvelopeSignature{{
			KeyID: Ed25519KeyID(publicKey),
			Sig:   base64.StdEncoding.EncodeToString(signature),
		}},
	}
}

// Marshal returns the canonical bytes of the envelope, the content of a
// `.dsse.json` file.
func (e Envelope) Marshal() ([]byte, error) {
	return Canonicalize(e)
}

// StatementBytes returns the statement the envelope carries.
func (e Envelope) StatementBytes() ([]byte, error) {
	payload, err := base64.StdEncoding.DecodeString(e.Payload)
	if err != nil {
		return nil, fmt.Errorf("%w: payload: %v", ErrEnvelopeShape, err)
	}
	return payload, nil
}

// SignatureBytes returns the raw bytes of a signature.
func (s EnvelopeSignature) SignatureBytes() ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(s.Sig)
	if err != nil {
		return nil, fmt.Errorf("%w: signature: %v", ErrEnvelopeShape, err)
	}
	return raw, nil
}

// DecodeEnvelope reads a DSSE envelope. It requires the in-toto payload type, a
// payload and at least one signature carrying a key identifier; it does not check
// any signature.
func DecodeEnvelope(data []byte) (Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Envelope{}, fmt.Errorf("%w: %v", ErrEnvelopeShape, err)
	}
	if envelope.PayloadType != DSSEPayloadType {
		return Envelope{}, fmt.Errorf("%w: payload type %q is not %q", ErrEnvelopeShape, envelope.PayloadType, DSSEPayloadType)
	}
	if _, err := envelope.StatementBytes(); err != nil {
		return Envelope{}, err
	}
	if len(envelope.Signatures) == 0 {
		return Envelope{}, fmt.Errorf("%w: no signature", ErrEnvelopeShape)
	}
	for index, signature := range envelope.Signatures {
		if signature.KeyID == "" {
			return Envelope{}, fmt.Errorf("%w: signature %d has no key identifier", ErrEnvelopeShape, index)
		}
		if _, err := signature.SignatureBytes(); err != nil {
			return Envelope{}, err
		}
	}
	return envelope, nil
}
