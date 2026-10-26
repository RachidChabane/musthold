// Package ed25519verifier verifies DSSE envelopes signed with Ed25519 keys pinned
// in the trust policy. It uses the standard library only: the DSSE
// pre-authentication encoding of the payload is the message the signature covers.
package ed25519verifier

import (
	"crypto/ed25519"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/verify/internal/domain/refusal"
	"github.com/RachidChabane/musthold/verify/internal/domain/trustpolicy"
)

// Verifier accepts an envelope that carries a valid signature of a pinned key.
type Verifier struct {
	policy trustpolicy.Policy
}

// New returns the verifier of a policy.
func New(policy trustpolicy.Policy) *Verifier {
	return &Verifier{policy: policy}
}

// Verify checks the envelope: its payload type, and at least one signature that a
// key of the policy made over the pre-authentication encoding of the payload. A
// signature by an unknown key counts for nothing, and an envelope whose only
// signatures are unknown or invalid is refused. It returns the statement bytes.
func (v *Verifier) Verify(bundle []byte) ([]byte, error) {
	envelope, err := format.DecodeEnvelope(bundle)
	if err != nil {
		return nil, refusal.Wrap(err)
	}
	statement, err := envelope.StatementBytes()
	if err != nil {
		return nil, refusal.Wrap(err)
	}
	message := format.PAE(envelope.PayloadType, statement)
	for _, signature := range envelope.Signatures {
		key, trusted := v.policy.Ed25519Key(signature.KeyID)
		if !trusted {
			continue
		}
		raw, err := signature.SignatureBytes()
		if err != nil {
			return nil, refusal.Wrap(err)
		}
		if ed25519.Verify(key.Public, message, raw) {
			return statement, nil
		}
	}
	return nil, refusal.Newf("no signature of a key pinned in the trust policy verifies")
}
