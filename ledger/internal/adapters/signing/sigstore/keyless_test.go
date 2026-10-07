package sigstore_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/secure-systems-lab/go-securesystemslib/dsse"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	protorekor "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"github.com/sigstore/sigstore-go/pkg/testing/ca"
	"github.com/sigstore/sigstore-go/pkg/tlog"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/sigstore/sigstore/pkg/cryptoutils"

	"github.com/RachidChabane/spec-proof/ledger/internal/adapters/signing/sigstore"
)

const (
	verifiesKeyless = " [verifies SP-153~1]"

	// The Rekor entry type and version of a DSSE log entry.
	rekorKind    = "dsse"
	rekorVersion = "0.0.1"

	testIdentity = "ci@example.test"
	testIssuer   = "https://issuer.example.test"
)

// virtualSigstore backs the keyless signer with the virtual Sigstore of
// sigstore-go: a Fulcio that issues a leaf certificate for the keypair and a
// Rekor that returns a signed entry.
type virtualSigstore struct {
	authority *ca.VirtualSigstore
	leaves    map[*ecdsa.PublicKey][]byte
}

// leafKeypair is the ECDSA key of a leaf certificate of the virtual Fulcio.
type leafKeypair struct{ key *ecdsa.PrivateKey }

func (leafKeypair) GetHashAlgorithm() protocommon.HashAlgorithm {
	return protocommon.HashAlgorithm_SHA2_256
}
func (leafKeypair) GetSigningAlgorithm() protocommon.PublicKeyDetails {
	return protocommon.PublicKeyDetails_PKIX_ECDSA_P256_SHA_256
}
func (leafKeypair) GetHint() []byte                  { return nil }
func (leafKeypair) GetKeyAlgorithm() string          { return "ECDSA" }
func (k leafKeypair) GetPublicKey() crypto.PublicKey { return &k.key.PublicKey }
func (k leafKeypair) GetPublicKeyPem() (string, error) {
	pemBytes, err := cryptoutils.MarshalPublicKeyToPEM(&k.key.PublicKey)
	return string(pemBytes), err
}
func (k leafKeypair) SignData(_ context.Context, data []byte) ([]byte, []byte, error) {
	digest := sha256.Sum256(data)
	signature, err := ecdsa.SignASN1(rand.Reader, k.key, digest[:])
	return signature, digest[:], err
}

func (v *virtualSigstore) newKeypair() (sign.Keypair, error) {
	certificate, privateKey, err := v.authority.GenerateLeafCert(testIdentity, testIssuer)
	if err != nil {
		return nil, err
	}
	key, _ := privateKey.(*ecdsa.PrivateKey)
	v.leaves[&key.PublicKey] = certificate.Raw
	return leafKeypair{key: key}, nil
}

// GetCertificate is the CertificateProvider of the virtual Fulcio.
func (v *virtualSigstore) GetCertificate(_ context.Context, keypair sign.Keypair, options *sign.CertificateProviderOptions) ([]byte, error) {
	if options == nil || options.IDToken == "" {
		return nil, errors.New("no identity token")
	}
	public, _ := keypair.GetPublicKey().(*ecdsa.PublicKey)
	return v.leaves[public], nil
}

// GetTransparencyLogEntry is the Transparency of the virtual Rekor.
func (v *virtualSigstore) GetTransparencyLogEntry(_ context.Context, _ []byte, b *protobundle.Bundle) error {
	certificate, err := x509.ParseCertificate(b.GetVerificationMaterial().GetCertificate().GetRawBytes())
	if err != nil {
		return err
	}
	envelope := b.GetDsseEnvelope()
	signature := envelope.GetSignatures()[0].GetSig()
	entry, err := v.authority.GenerateTlogEntry(certificate, &dsse.Envelope{
		PayloadType: envelope.GetPayloadType(),
		Payload:     base64.StdEncoding.EncodeToString(envelope.GetPayload()),
		Signatures:  []dsse.Signature{{Sig: base64.StdEncoding.EncodeToString(signature)}},
	}, signature, time.Now().Add(5*time.Minute).Unix(), true)
	if err != nil {
		return err
	}
	logEntry := entry.TransparencyLogEntry()
	signedEntryTimestamp, err := v.authority.RekorSignPayload(tlog.RekorPayload{
		Body:           base64.StdEncoding.EncodeToString(logEntry.GetCanonicalizedBody()),
		IntegratedTime: logEntry.GetIntegratedTime(),
		LogIndex:       logEntry.GetLogIndex(),
		LogID:          hex.EncodeToString(logEntry.GetLogId().GetKeyId()),
	})
	if err != nil {
		return err
	}
	logEntry.KindVersion = &protorekor.KindVersion{Kind: rekorKind, Version: rekorVersion}
	logEntry.InclusionPromise = &protorekor.InclusionPromise{SignedEntryTimestamp: signedEntryTimestamp}
	b.VerificationMaterial.TlogEntries = append(b.VerificationMaterial.TlogEntries, logEntry)
	return nil
}

func newVirtualSigstore(t *testing.T) *virtualSigstore {
	t.Helper()
	authority, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatal(err)
	}
	return &virtualSigstore{authority: authority, leaves: map[*ecdsa.PublicKey][]byte{}}
}

func (v *virtualSigstore) signer() *sigstore.Signer {
	return sigstore.NewSigner(v.newKeypair, sign.BundleOptions{
		CertificateProvider:        v,
		CertificateProviderOptions: &sign.CertificateProviderOptions{IDToken: "token"},
		TransparencyLogs:           []sign.Transparency{v},
	})
}

func (v *virtualSigstore) verifyIdentity(t *testing.T, bundleJSON []byte) *verify.VerificationResult {
	t.Helper()
	parsed := &bundle.Bundle{}
	if err := parsed.UnmarshalJSON(bundleJSON); err != nil {
		t.Fatal(err)
	}
	verifier, err := verify.NewVerifier(v.authority, verify.WithTransparencyLog(1), verify.WithIntegratedTimestamps(1))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := verify.NewShortCertificateIdentity(testIssuer, "", testIdentity, "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := verifier.Verify(parsed, verify.NewPolicy(verify.WithoutArtifactUnsafe(), verify.WithCertificateIdentity(identity)))
	if err != nil {
		t.Fatalf("the bundle does not verify against the virtual Sigstore: %v", err)
	}
	return result
}

func TestKeylessSigner(t *testing.T) {
	statement := []byte(`{"_type":"https://in-toto.io/Statement/v1","subject":[],"predicateType":"x","predicate":{}}`)

	t.Run("a statement is signed with a certificate and a log entry and verifies under its identity"+verifiesKeyless, func(t *testing.T) {
		virtual := newVirtualSigstore(t)
		bundleJSON, err := virtual.signer().Sign(statement)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := sigstore.Payload(bundleJSON)
		if err != nil || string(payload) != string(statement) {
			t.Fatalf("payload %q: %v", payload, err)
		}
		virtual.verifyIdentity(t, bundleJSON)
	})

	t.Run("each statement gets a fresh ephemeral key and certificate"+verifiesKeyless, func(t *testing.T) {
		virtual := newVirtualSigstore(t)
		signer := virtual.signer()
		certificates := map[string]bool{}
		for range 3 {
			bundleJSON, err := signer.Sign(statement)
			if err != nil {
				t.Fatal(err)
			}
			var view struct {
				VerificationMaterial struct {
					Certificate struct {
						RawBytes string `json:"rawBytes"`
					} `json:"certificate"`
					TlogEntries []json.RawMessage `json:"tlogEntries"`
				} `json:"verificationMaterial"`
			}
			if err := json.Unmarshal(bundleJSON, &view); err != nil {
				t.Fatal(err)
			}
			if view.VerificationMaterial.Certificate.RawBytes == "" || len(view.VerificationMaterial.TlogEntries) != 1 {
				t.Fatalf("verification material %+v", view.VerificationMaterial)
			}
			certificates[view.VerificationMaterial.Certificate.RawBytes] = true
		}
		if len(certificates) != 3 {
			t.Fatalf("%d distinct certificates for 3 statements", len(certificates))
		}
	})

	t.Run("a bundle signed by another authority does not verify against the trusted one"+verifiesKeyless, func(t *testing.T) {
		signing, trusted := newVirtualSigstore(t), newVirtualSigstore(t)
		bundleJSON, err := signing.signer().Sign(statement)
		if err != nil {
			t.Fatal(err)
		}
		parsed := &bundle.Bundle{}
		if err := parsed.UnmarshalJSON(bundleJSON); err != nil {
			t.Fatal(err)
		}
		verifier, err := verify.NewVerifier(trusted.authority, verify.WithTransparencyLog(1), verify.WithIntegratedTimestamps(1))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := verifier.Verify(parsed, verify.NewPolicy(verify.WithoutArtifactUnsafe(), verify.WithoutIdentitiesUnsafe())); err == nil {
			t.Fatal("a bundle of an untrusted authority verified")
		}
	})

	t.Run("an empty statement is refused"+verifiesKeyless, func(t *testing.T) {
		if _, err := newVirtualSigstore(t).signer().Sign(nil); !errors.Is(err, sigstore.ErrEmptyStatement) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a failing keypair factory is reported"+verifiesKeyless, func(t *testing.T) {
		failing := sigstore.NewSigner(func() (sign.Keypair, error) { return nil, errors.New("no entropy") }, sign.BundleOptions{})
		if _, err := failing.Sign(statement); err == nil {
			t.Fatal("signed without a keypair")
		}
	})

	t.Run("keyless signing refuses to start without an identity token"+verifiesKeyless, func(t *testing.T) {
		if _, err := sigstore.NewKeyless(sigstore.KeylessConfig{FulcioURL: sigstore.PublicFulcioURL, RekorURL: sigstore.PublicRekorURL}); !errors.Is(err, sigstore.ErrNoIDToken) {
			t.Fatalf("error %v", err)
		}
		if _, err := sigstore.NewKeyless(sigstore.KeylessConfig{FulcioURL: sigstore.PublicFulcioURL, RekorURL: sigstore.PublicRekorURL, IDToken: "token"}); err != nil {
			t.Fatal(err)
		}
	})
}

func lookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, found := values[name]
		return value, found
	}
}

func TestTokenSource(t *testing.T) {
	t.Run("an explicit token wins"+verifiesKeyless, func(t *testing.T) {
		source := sigstore.TokenSource{LookupEnv: lookup(map[string]string{sigstore.IDTokenEnvironmentVariable: "explicit"})}
		if token, err := source.Token(context.Background()); err != nil || token != "explicit" {
			t.Fatalf("token %q: %v", token, err)
		}
	})

	t.Run("the GitHub Actions token service is asked for the sigstore audience"+verifiesKeyless, func(t *testing.T) {
		var gotAuthorization, gotAudience string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuthorization, gotAudience = r.Header.Get("Authorization"), r.URL.Query().Get("audience")
			_ = json.NewEncoder(w).Encode(map[string]string{"value": "from-actions"})
		}))
		defer server.Close()
		source := sigstore.TokenSource{
			LookupEnv: lookup(map[string]string{sigstore.ActionsRequestURLVariable: server.URL + "/?api=1", sigstore.ActionsRequestTokenVariable: "request-token"}),
			Client:    server.Client(),
		}
		token, err := source.Token(context.Background())
		if err != nil || token != "from-actions" {
			t.Fatalf("token %q: %v", token, err)
		}
		if gotAuthorization != "Bearer request-token" || gotAudience != sigstore.IDTokenAudience {
			t.Fatalf("authorization %q audience %q", gotAuthorization, gotAudience)
		}
	})

	t.Run("no token anywhere is refused"+verifiesKeyless, func(t *testing.T) {
		for name, values := range map[string]map[string]string{
			"nothing":        {},
			"url only":       {sigstore.ActionsRequestURLVariable: "http://x"},
			"empty explicit": {sigstore.IDTokenEnvironmentVariable: ""},
		} {
			if _, err := (sigstore.TokenSource{LookupEnv: lookup(values)}).Token(context.Background()); !errors.Is(err, sigstore.ErrNoIDToken) {
				t.Errorf("%s: error %v", name, err)
			}
		}
	})

	t.Run("a failing or empty token service is reported"+verifiesKeyless, func(t *testing.T) {
		for name, handler := range map[string]http.HandlerFunc{
			"status":   func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) },
			"no value": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) },
			"not JSON": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`<`)) },
		} {
			server := httptest.NewServer(handler)
			source := sigstore.TokenSource{
				LookupEnv: lookup(map[string]string{sigstore.ActionsRequestURLVariable: server.URL, sigstore.ActionsRequestTokenVariable: "t"}),
				Client:    server.Client(),
			}
			if _, err := source.Token(context.Background()); err == nil {
				t.Errorf("%s: accepted", name)
			}
			server.Close()
		}
	})
}
