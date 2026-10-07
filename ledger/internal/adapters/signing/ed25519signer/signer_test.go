package ed25519signer_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/adapters/signing/ed25519signer"
	"github.com/RachidChabane/spec-proof/ledger/internal/testkit"
)

const verifiesEd25519 = " [verifies SP-152~1]"

func noEnvironment(string) (string, bool) { return "", false }

func environmentWith(value string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		if name == ed25519signer.KeyEnvironmentVariable {
			return value, true
		}
		return "", false
	}
}

func TestSign(t *testing.T) {
	identity := testkit.NewEd25519Identity(t, 1)
	signer, err := ed25519signer.New(identity.Private)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the signature covers the pre-authentication encoding of the exact bytes"+verifiesEd25519, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			statement := rapid.SliceOfN(rapid.Byte(), 1, 256).Draw(t, "statement")
			bundle, err := signer.Sign(statement)
			if err != nil {
				t.Fatal(err)
			}
			envelope, err := format.DecodeEnvelope(bundle)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := envelope.StatementBytes()
			if err != nil || string(payload) != string(statement) {
				t.Fatalf("payload differs: %v", err)
			}
			signature, err := envelope.Signatures[0].SignatureBytes()
			if err != nil || !ed25519.Verify(identity.Public, format.PAE(format.DSSEPayloadType, statement), signature) {
				t.Fatalf("signature does not verify: %v", err)
			}
			if envelope.Signatures[0].KeyID != format.Ed25519KeyID(identity.Public) {
				t.Fatal("wrong key identifier")
			}
		})
	})

	t.Run("an empty statement is refused"+verifiesEd25519, func(t *testing.T) {
		if _, err := signer.Sign(nil); err == nil {
			t.Fatal("signed nothing")
		}
	})

	t.Run("a key of the wrong size is refused"+verifiesEd25519, func(t *testing.T) {
		if _, err := ed25519signer.New(ed25519.PrivateKey{1, 2, 3}); !errors.Is(err, ed25519signer.ErrKeyFormat) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestLoadKey(t *testing.T) {
	identity := testkit.NewEd25519Identity(t, 2)

	t.Run("the key comes from the environment variable"+verifiesEd25519, func(t *testing.T) {
		key, err := ed25519signer.KeySource{LookupEnv: environmentWith(string(identity.PEM))}.Load()
		if err != nil || !key.Equal(identity.Private) {
			t.Fatalf("key %v: %v", key, err)
		}
	})

	t.Run("the key comes from a file only its owner can read"+verifiesEd25519, func(t *testing.T) {
		path := identity.WriteKeyFile(t, t.TempDir())
		key, err := ed25519signer.KeySource{File: path, LookupEnv: noEnvironment}.Load()
		if err != nil || !key.Equal(identity.Private) {
			t.Fatalf("key %v: %v", key, err)
		}
	})

	t.Run("a key file that group or others can read is refused"+verifiesEd25519, func(t *testing.T) {
		for _, mode := range []os.FileMode{0o640, 0o604, 0o644, 0o660} {
			path := identity.WriteKeyFile(t, t.TempDir())
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			_, err := ed25519signer.KeySource{File: path, LookupEnv: noEnvironment}.Load()
			if !errors.Is(err, ed25519signer.ErrKeyFilePermission) {
				t.Errorf("mode %o: error %v", mode, err)
			}
		}
	})

	t.Run("a key given both ways is refused"+verifiesEd25519, func(t *testing.T) {
		path := identity.WriteKeyFile(t, t.TempDir())
		_, err := ed25519signer.KeySource{File: path, LookupEnv: environmentWith(string(identity.PEM))}.Load()
		if !errors.Is(err, ed25519signer.ErrAmbiguousKey) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a missing key is refused"+verifiesEd25519, func(t *testing.T) {
		for _, lookup := range []func(string) (string, bool){noEnvironment, environmentWith("")} {
			if _, err := (ed25519signer.KeySource{LookupEnv: lookup}).Load(); !errors.Is(err, ed25519signer.ErrNoKey) {
				t.Errorf("error %v", err)
			}
		}
	})

	t.Run("a missing key file is a failure"+verifiesEd25519, func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "absent.pem")
		if _, err := (ed25519signer.KeySource{File: missing, LookupEnv: noEnvironment}).Load(); err == nil {
			t.Fatal("loaded a key from nowhere")
		}
	})

	t.Run("a key that is not an Ed25519 PKCS#8 PEM is refused"+verifiesEd25519, func(t *testing.T) {
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		der, err := x509.MarshalPKCS8PrivateKey(rsaKey)
		if err != nil {
			t.Fatal(err)
		}
		cases := map[string]string{
			"not PEM":     "plain text",
			"wrong block": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{1}})),
			"bad bytes":   string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1, 2}})),
			"RSA key":     string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		}
		for name, value := range cases {
			if _, err := (ed25519signer.KeySource{LookupEnv: environmentWith(value)}).Load(); !errors.Is(err, ed25519signer.ErrKeyFormat) {
				t.Errorf("%s: error %v", name, err)
			}
		}
	})
}

func FuzzLoadKey(f *testing.F) {
	identity := testkit.NewEd25519Identity(f, 3)
	f.Add(identity.PEM)
	f.Add([]byte("-----BEGIN PRIVATE KEY-----\n-----END PRIVATE KEY-----"))
	f.Fuzz(func(t *testing.T, data []byte) {
		key, err := ed25519signer.KeySource{LookupEnv: environmentWith(string(data))}.Load()
		if err == nil && len(key) != ed25519.PrivateKeySize {
			t.Fatalf("accepted a key of %d bytes", len(key))
		}
	})
}
