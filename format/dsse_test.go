package format_test

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
)

const verifiesDSSE = " [verifies SP-199~1]"

func testKey(t testing.TB) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	seed := bytes.Repeat([]byte{7}, ed25519.SeedSize)
	private := ed25519.NewKeyFromSeed(seed)
	return private.Public().(ed25519.PublicKey), private
}

func TestPAE(t *testing.T) {
	t.Run("the encoding is the one of the DSSE specification"+verifiesDSSE, func(t *testing.T) {
		got := format.PAE("http://example.com/HelloWorld", []byte("hello world"))
		want := "DSSEv1 29 http://example.com/HelloWorld 11 hello world"
		if string(got) != want {
			t.Fatalf("PAE %q, want %q", got, want)
		}
	})

	t.Run("an empty type and payload are length-prefixed with zero"+verifiesDSSE, func(t *testing.T) {
		if got := format.PAE("", nil); string(got) != "DSSEv1 0  0 " {
			t.Fatalf("PAE %q", got)
		}
	})
}

func TestSignEnvelope(t *testing.T) {
	public, private := testKey(t)
	statement := []byte(`{"a":"b"}`)

	t.Run("the signature covers the pre-authentication encoding of the exact bytes"+verifiesDSSE, func(t *testing.T) {
		envelope := format.SignEnvelope(statement, private)
		if envelope.PayloadType != format.DSSEPayloadType || len(envelope.Signatures) != 1 {
			t.Fatalf("envelope %+v", envelope)
		}
		signature, err := envelope.Signatures[0].SignatureBytes()
		if err != nil || !ed25519.Verify(public, format.PAE(format.DSSEPayloadType, statement), signature) {
			t.Fatalf("signature does not verify: %v", err)
		}
		if envelope.Signatures[0].KeyID != format.Ed25519KeyID(public) {
			t.Fatal("key identifier is not the digest of the public key")
		}
	})

	t.Run("an envelope round-trips through its canonical bytes"+verifiesDSSE, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			payload := rapid.SliceOf(rapid.Byte()).Draw(t, "payload")
			encoded, err := format.SignEnvelope(payload, private).Marshal()
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := format.DecodeEnvelope(encoded)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decoded.StatementBytes()
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("payload differs: %v", err)
			}
			again, err := decoded.Marshal()
			if err != nil || !bytes.Equal(again, encoded) {
				t.Fatal("canonical bytes are not stable")
			}
		})
	})
}

func TestShapeOfBundle(t *testing.T) {
	cases := []struct {
		name     string
		document string
		want     format.BundleShape
	}{
		{"an envelope", `{"payloadType":"application/vnd.in-toto+json","payload":"e30=","signatures":[]}`, format.ShapeEnvelope},
		{"a Sigstore bundle by media type", `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json"}`, format.ShapeSigstoreBundle},
		{"a Sigstore bundle by envelope", `{"dsseEnvelope":{}}`, format.ShapeSigstoreBundle},
		{"another document", `{"hello":"world"}`, format.ShapeUnknown},
		{"not JSON", `{`, format.ShapeUnknown},
		{"an array", `[]`, format.ShapeUnknown},
	}
	for _, tt := range cases {
		t.Run(tt.name+verifiesDSSE, func(t *testing.T) {
			if got := format.ShapeOfBundle([]byte(tt.document)); got != tt.want {
				t.Fatalf("shape %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDecodeEnvelopeRefusals(t *testing.T) {
	cases := []struct{ name, document string }{
		{"not JSON", `{`},
		{"another payload type", `{"payloadType":"text/plain","payload":"e30=","signatures":[{"keyid":"k","sig":"AA=="}]}`},
		{"a payload that is not base64", `{"payloadType":"application/vnd.in-toto+json","payload":"***","signatures":[{"keyid":"k","sig":"AA=="}]}`},
		{"no signature", `{"payloadType":"application/vnd.in-toto+json","payload":"e30=","signatures":[]}`},
		{"a signature without key identifier", `{"payloadType":"application/vnd.in-toto+json","payload":"e30=","signatures":[{"sig":"AA=="}]}`},
		{"a signature that is not base64", `{"payloadType":"application/vnd.in-toto+json","payload":"e30=","signatures":[{"keyid":"k","sig":"***"}]}`},
	}
	for _, tt := range cases {
		t.Run(tt.name+" is refused"+verifiesDSSE, func(t *testing.T) {
			_, err := format.DecodeEnvelope([]byte(tt.document))
			if err == nil {
				t.Fatal("accepted")
			}
			if tt.name != "not JSON" && !errors.Is(err, format.ErrEnvelopeShape) {
				t.Fatalf("error %v", err)
			}
		})
	}
	t.Run("the key identifier is a SHA-256 hex digest"+verifiesDSSE, func(t *testing.T) {
		public, _ := testKey(t)
		if id := format.Ed25519KeyID(public); !format.IsSHA256Hex(id) || strings.ToLower(id) != id {
			t.Fatalf("id %q", id)
		}
	})
}
