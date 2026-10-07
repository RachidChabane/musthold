package ed25519verifier_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/verify/internal/adapters/policyfile"
	"github.com/RachidChabane/spec-proof/verify/internal/adapters/signing/ed25519verifier"
	"github.com/RachidChabane/spec-proof/verify/internal/domain/refusal"
	"github.com/RachidChabane/spec-proof/verify/internal/testkit"
)

const verifiesEd25519 = " [verifies SP-211~1]"

func verifierPinning(t testing.TB, identities ...testkit.Identity) *ed25519verifier.Verifier {
	t.Helper()
	policy, err := policyfile.Parse(testkit.PolicyYAML(identities...))
	if err != nil {
		t.Fatal(err)
	}
	return ed25519verifier.New(policy)
}

// fataler is what both *testing.T and *rapid.T offer.
type fataler interface {
	Helper()
	Fatal(args ...any)
}

func marshal(t fataler, envelope format.Envelope) []byte {
	t.Helper()
	encoded, err := envelope.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestVerify(t *testing.T) {
	signer, other := testkit.NewIdentity(1), testkit.NewIdentity(2)
	verifier := verifierPinning(t, signer)

	t.Run("a signature of a pinned key over the exact statement bytes verifies and gives the statement"+verifiesEd25519, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			statement := rapid.SliceOfN(rapid.Byte(), 1, 200).Draw(t, "statement")
			got, err := verifier.Verify(marshal(t, format.SignEnvelope(statement, signer.Private)))
			if err != nil || string(got) != string(statement) {
				t.Fatalf("statement %q: %v", got, err)
			}
		})
	})

	t.Run("a key the policy does not pin is refused"+verifiesEd25519, func(t *testing.T) {
		_, err := verifier.Verify(marshal(t, format.SignEnvelope([]byte("s"), other.Private)))
		if !refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a tampered payload or signature is refused"+verifiesEd25519, func(t *testing.T) {
		envelope := format.SignEnvelope([]byte("statement"), signer.Private)
		tamperedPayload := envelope
		tamperedPayload.Payload = base64.StdEncoding.EncodeToString([]byte("statemenT"))
		raw, _ := envelope.Signatures[0].SignatureBytes()
		raw[0] ^= 0xff
		tamperedSignature := envelope
		tamperedSignature.Signatures = []format.EnvelopeSignature{{KeyID: envelope.Signatures[0].KeyID, Sig: base64.StdEncoding.EncodeToString(raw)}}
		for name, tampered := range map[string]format.Envelope{"payload": tamperedPayload, "signature": tamperedSignature} {
			if _, err := verifier.Verify(marshal(t, tampered)); !refusal.Is(err) {
				t.Errorf("%s: error %v", name, err)
			}
		}
	})

	t.Run("a signature over the payload without the pre-authentication encoding is refused"+verifiesEd25519, func(t *testing.T) {
		statement := []byte("statement")
		envelope := format.SignEnvelope(statement, signer.Private)
		envelope.Signatures[0].Sig = base64.StdEncoding.EncodeToString(ed25519.Sign(signer.Private, statement))
		if _, err := verifier.Verify(marshal(t, envelope)); !refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("an envelope whose payload type was changed is refused"+verifiesEd25519, func(t *testing.T) {
		envelope := format.SignEnvelope([]byte("statement"), signer.Private)
		envelope.PayloadType = "text/plain"
		if _, err := verifier.Verify(marshal(t, envelope)); !refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("an unknown signature next to a valid one does not hide it"+verifiesEd25519, func(t *testing.T) {
		envelope := format.SignEnvelope([]byte("statement"), signer.Private)
		stranger := format.SignEnvelope([]byte("statement"), other.Private)
		envelope.Signatures = append([]format.EnvelopeSignature{stranger.Signatures[0]}, envelope.Signatures...)
		if _, err := verifier.Verify(marshal(t, envelope)); err != nil {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a valid signature of an unknown key next to a bad one of a pinned key is refused"+verifiesEd25519, func(t *testing.T) {
		envelope := format.SignEnvelope([]byte("statement"), signer.Private)
		raw, _ := envelope.Signatures[0].SignatureBytes()
		raw[1] ^= 0xff
		envelope.Signatures[0].Sig = base64.StdEncoding.EncodeToString(raw)
		envelope.Signatures = append(envelope.Signatures, format.SignEnvelope([]byte("statement"), other.Private).Signatures[0])
		if _, err := verifier.Verify(marshal(t, envelope)); !refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a document that is not an envelope is refused"+verifiesEd25519, func(t *testing.T) {
		for _, document := range []string{`{`, `{}`, `{"payloadType":"application/vnd.in-toto+json","payload":"e30=","signatures":[]}`} {
			if _, err := verifier.Verify([]byte(document)); !refusal.Is(err) {
				t.Errorf("%s: error %v", document, err)
			}
		}
	})

	t.Run("a policy that pins several keys accepts each of them"+verifiesEd25519, func(t *testing.T) {
		both := verifierPinning(t, signer, other)
		for _, identity := range []testkit.Identity{signer, other} {
			if _, err := both.Verify(marshal(t, format.SignEnvelope([]byte("s"), identity.Private))); err != nil {
				t.Errorf("error %v", err)
			}
		}
	})
}

func FuzzVerify(f *testing.F) {
	signer := testkit.NewIdentity(1)
	policy, _ := policyfile.Parse(testkit.PolicyYAML(signer))
	verifier := ed25519verifier.New(policy)
	valid, _ := format.SignEnvelope([]byte("statement"), signer.Private).Marshal()
	f.Add(valid)
	f.Fuzz(func(t *testing.T, bundle []byte) {
		statement, err := verifier.Verify(bundle)
		if err != nil {
			return
		}
		var envelope format.Envelope
		if json.Unmarshal(bundle, &envelope) != nil {
			t.Fatal("accepted a document that is not JSON")
		}
		payload, _ := envelope.StatementBytes()
		if string(payload) != string(statement) {
			t.Fatal("returned a statement other than the payload")
		}
	})
}
