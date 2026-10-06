package sigstore_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/sign"
	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/internal/adapters/signing/sigstore"
)

const verifiesSP48 = " [verifies SP-48~1]"

const largeStatementRepeats = 100_000

func newKeypair(t testing.TB) sign.Keypair {
	t.Helper()
	keypair, err := sign.NewEphemeralKeypair(nil)
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}
	return keypair
}

type envelopeView struct {
	DsseEnvelope struct {
		Payload     string `json:"payload"`
		PayloadType string `json:"payloadType"`
		Signatures  []struct {
			Sig string `json:"sig"`
		} `json:"signatures"`
	} `json:"dsseEnvelope"`
}

func decodeEnvelope(t *testing.T, bundleJSON []byte) envelopeView {
	t.Helper()
	var view envelopeView
	if err := json.Unmarshal(bundleJSON, &view); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	return view
}

func TestWrapThenPayloadIsByteIdentical(t *testing.T) {
	tests := []struct {
		name      string
		statement []byte
	}{
		{"compact in-toto statement", []byte(`{"_type":"https://in-toto.io/Statement/v1","subject":[],"predicateType":"x","predicate":{}}`)},
		{"non-ascii", []byte(`{"name":"résumé ✓ 日本語"}`)},
		{"binary bytes", []byte{0x00, 0xff, 0xfe, 0x10, 0x80}},
		{"large", bytes.Repeat([]byte(`{"k":"vvvvvvvv"},`), largeStatementRepeats)},
		{"single byte", []byte{'{'}},
	}
	keypair := newKeypair(t)
	for _, tt := range tests {
		t.Run(tt.name+verifiesSP48, func(t *testing.T) {
			bundleJSON, err := sigstore.Wrap(tt.statement, keypair)
			if err != nil {
				t.Fatalf("wrap: %v", err)
			}
			payload, err := sigstore.Payload(bundleJSON)
			if err != nil {
				t.Fatalf("payload: %v", err)
			}
			if !bytes.Equal(payload, tt.statement) {
				t.Fatal("payload differs from statement")
			}
		})
	}
}

func TestWrapProducesDSSEEnvelope(t *testing.T) {
	statement := []byte(`{"a":1}`)
	bundleJSON, err := sigstore.Wrap(statement, newKeypair(t))
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	view := decodeEnvelope(t, bundleJSON)
	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{"payload type is the in-toto constant", func(t *testing.T) {
			if view.DsseEnvelope.PayloadType != sigstore.PayloadType {
				t.Fatalf("payloadType = %q", view.DsseEnvelope.PayloadType)
			}
		}},
		{"envelope carries exactly one non-empty signature", func(t *testing.T) {
			signatures := view.DsseEnvelope.Signatures
			if len(signatures) != 1 || signatures[0].Sig == "" {
				t.Fatalf("signatures = %+v", signatures)
			}
		}},
		{"envelope payload is the base64 of the statement", func(t *testing.T) {
			decoded, err := base64.StdEncoding.DecodeString(view.DsseEnvelope.Payload)
			if err != nil || !bytes.Equal(decoded, statement) {
				t.Fatalf("payload = %q, err = %v", decoded, err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name+verifiesSP48, tt.check)
	}
}

func TestWrapRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		statement []byte
		keypair   sign.Keypair
		want      error
	}{
		{"empty statement", nil, newKeypair(t), sigstore.ErrEmptyStatement},
		{"missing keypair", []byte("{}"), nil, sigstore.ErrNoKeypair},
	}
	for _, tt := range tests {
		t.Run(tt.name+verifiesSP48, func(t *testing.T) {
			_, err := sigstore.Wrap(tt.statement, tt.keypair)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestPayloadRejectsMalformedBundles(t *testing.T) {
	tests := []struct {
		name   string
		bundle string
	}{
		{"not json", "not json"},
		{"empty object", "{}"},
		{"unknown media type", `{"mediaType":"application/x-unknown"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name+verifiesSP48, func(t *testing.T) {
			if _, err := sigstore.Payload([]byte(tt.bundle)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestWrapPayloadRoundTripProperty(t *testing.T) {
	const maxStatementBytes = 4096
	keypair := newKeypair(t)
	t.Run("any non-empty payload survives the round trip"+verifiesSP48, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			statement := rapid.SliceOfN(rapid.Byte(), 1, maxStatementBytes).Draw(t, "statement")
			bundleJSON, err := sigstore.Wrap(statement, keypair)
			if err != nil {
				t.Fatalf("wrap: %v", err)
			}
			payload, err := sigstore.Payload(bundleJSON)
			if err != nil {
				t.Fatalf("payload: %v", err)
			}
			if !bytes.Equal(payload, statement) {
				t.Fatal("round trip changed the payload")
			}
		})
	})
}
