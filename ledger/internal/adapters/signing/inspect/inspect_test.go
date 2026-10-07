package inspect_test

import (
	"errors"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/sign"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/adapters/signing/ed25519signer"
	"github.com/RachidChabane/spec-proof/ledger/internal/adapters/signing/inspect"
	"github.com/RachidChabane/spec-proof/ledger/internal/adapters/signing/sigstore"
	"github.com/RachidChabane/spec-proof/ledger/internal/testkit"
)

const verifiesInspect = " [verifies SP-154~1]"

var statement = []byte(`{"_type":"https://in-toto.io/Statement/v1","subject":[],"predicateType":"x","predicate":{}}`)

func TestPayload(t *testing.T) {
	t.Run("the payload of an Ed25519 envelope is the statement"+verifiesInspect, func(t *testing.T) {
		signer, err := ed25519signer.New(testkit.NewEd25519Identity(t, 1).Private)
		if err != nil {
			t.Fatal(err)
		}
		bundle, err := signer.Sign(statement)
		if err != nil {
			t.Fatal(err)
		}
		got, err := inspect.Reader{}.Payload(bundle)
		if err != nil || string(got) != string(statement) {
			t.Fatalf("payload %q: %v", got, err)
		}
	})

	t.Run("the payload of a Sigstore bundle is the statement"+verifiesInspect, func(t *testing.T) {
		keypair, err := sign.NewEphemeralKeypair(nil)
		if err != nil {
			t.Fatal(err)
		}
		bundle, err := sigstore.Wrap(statement, keypair)
		if err != nil {
			t.Fatal(err)
		}
		got, err := inspect.Reader{}.Payload(bundle)
		if err != nil || string(got) != string(statement) {
			t.Fatalf("payload %q: %v", got, err)
		}
	})

	t.Run("a document of neither shape is refused"+verifiesInspect, func(t *testing.T) {
		if _, err := (inspect.Reader{}).Payload([]byte(`{"hello":"world"}`)); !errors.Is(err, inspect.ErrUnknownBundle) {
			t.Fatalf("error %v", err)
		}
		if _, err := (inspect.Reader{}).Payload([]byte(`not json`)); err == nil {
			t.Fatal("accepted")
		}
	})

	t.Run("an envelope without signature is refused"+verifiesInspect, func(t *testing.T) {
		_, err := inspect.Reader{}.Payload([]byte(`{"payloadType":"application/vnd.in-toto+json","payload":"e30=","signatures":[]}`))
		if !errors.Is(err, format.ErrEnvelopeShape) {
			t.Fatalf("error %v", err)
		}
	})
}

func FuzzPayload(f *testing.F) {
	signer, _ := ed25519signer.New(testkit.NewEd25519Identity(f, 1).Private)
	bundle, _ := signer.Sign(statement)
	f.Add(bundle)
	f.Add([]byte(`{"mediaType":"x"}`))
	f.Add([]byte(`{"dsseEnvelope":{}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = inspect.Reader{}.Payload(data)
	})
}
