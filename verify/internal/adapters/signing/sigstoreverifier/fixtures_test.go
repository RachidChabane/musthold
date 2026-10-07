package sigstoreverifier_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/secure-systems-lab/go-securesystemslib/dsse"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	protorekor "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	prototrustroot "github.com/sigstore/protobuf-specs/gen/pb-go/trustroot/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"github.com/sigstore/sigstore-go/pkg/testing/ca"
	"github.com/sigstore/sigstore-go/pkg/tlog"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/theupdateframework/go-tuf/v2/examples/repository/repository"
	"github.com/theupdateframework/go-tuf/v2/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/verify/internal/testkit"
)

// Layout of testdata, written by `go test -update-fixtures`.
const (
	testdataDirectory = "testdata"
	tufRootFile       = "tuf-root.json"
	snapshotDirectory = "snapshot"
	bundlesDirectory  = "bundles"

	fixtureValidity = 100 * 365 * 24 * time.Hour
	leafValidity    = 10 * time.Minute
	observedAfter   = 5 * time.Minute
)

var updateFixtures = flag.Bool("update-fixtures", false, "regenerate testdata with a virtual Sigstore")

// TestGenerateFixtures rewrites testdata when -update-fixtures is given. It runs
// before the tests that read testdata, which follow it in file order.
func TestGenerateFixtures(t *testing.T) {
	if !*updateFixtures {
		t.Skip("run with -update-fixtures to regenerate testdata")
	}
	if err := generateFixtures(testkit.ValidStatements(t)); err != nil {
		t.Fatal(err)
	}
}

// OIDs of the Fulcio certificate extensions.
var (
	oidIssuerV1          = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 1}
	oidIssuerV2          = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 8}
	oidBuildSignerURI    = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 9}
	oidBuildSignerDigest = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 10}
	utf8StringParameters = "utf8"
)

// The Rekor entry type and version of a DSSE log entry.
const (
	rekorKind    = "dsse"
	rekorVersion = "0.0.1"
)

// fulcio is a certificate authority of the fixtures that issues certificates with
// the build signer extensions of Fulcio, which the virtual Fulcio of sigstore-go
// does not.
type fulcio struct {
	root         *x509.Certificate
	intermediate *x509.Certificate
	key          *ecdsa.PrivateKey
}

func newFulcio() (*fulcio, error) {
	rootCertificate, rootKey, err := ca.GenerateRootCa()
	if err != nil {
		return nil, err
	}
	intermediate, intermediateKey, err := ca.GenerateFulcioIntermediate(rootCertificate, rootKey)
	if err != nil {
		return nil, err
	}
	return &fulcio{root: rootCertificate, intermediate: intermediate, key: intermediateKey}, nil
}

// issue returns a leaf certificate for key: a URI subject alternative name, the
// issuer and the build signer extensions.
func (f *fulcio) issue(key *ecdsa.PublicKey, issuer, signerURI, signerDigest string) (*x509.Certificate, error) {
	uri, err := url.Parse(signerURI)
	if err != nil {
		return nil, err
	}
	encode := func(value string) ([]byte, error) { return asn1.MarshalWithParams(value, utf8StringParameters) }
	issuerDER, err := encode(issuer)
	if err != nil {
		return nil, err
	}
	uriDER, err := encode(signerURI)
	if err != nil {
		return nil, err
	}
	digestDER, err := encode(signerDigest)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		NotBefore:    now,
		NotAfter:     now.Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		URIs:         []*url.URL{uri},
		ExtraExtensions: []pkix.Extension{
			{Id: oidIssuerV1, Value: []byte(issuer)},
			{Id: oidIssuerV2, Value: issuerDER},
			{Id: oidBuildSignerURI, Value: uriDER},
			{Id: oidBuildSignerDigest, Value: digestDER},
		},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, f.intermediate, key, f.key)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}

// leafKeypair is the ECDSA key a leaf certificate was issued for.
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

// issuedCertificate hands the certificate of one signature to sign.Bundle.
type issuedCertificate struct{ certificate *x509.Certificate }

func (i issuedCertificate) GetCertificate(context.Context, sign.Keypair, *sign.CertificateProviderOptions) ([]byte, error) {
	return i.certificate.Raw, nil
}

// rekor is the transparency log of the fixtures: the virtual Rekor of sigstore-go.
type rekor struct{ virtual *ca.VirtualSigstore }

func (r rekor) GetTransparencyLogEntry(_ context.Context, _ []byte, b *protobundle.Bundle) error {
	certificate, err := x509.ParseCertificate(b.GetVerificationMaterial().GetCertificate().GetRawBytes())
	if err != nil {
		return err
	}
	envelope := b.GetDsseEnvelope()
	signature := envelope.GetSignatures()[0].GetSig()
	entry, err := r.virtual.GenerateTlogEntry(certificate, &dsse.Envelope{
		PayloadType: envelope.GetPayloadType(),
		Payload:     base64.StdEncoding.EncodeToString(envelope.GetPayload()),
		Signatures:  []dsse.Signature{{Sig: base64.StdEncoding.EncodeToString(signature)}},
	}, signature, time.Now().Add(observedAfter).Unix(), true)
	if err != nil {
		return err
	}
	logEntry := entry.TransparencyLogEntry()
	signedEntryTimestamp, err := r.virtual.RekorSignPayload(tlog.RekorPayload{
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

// signStatement signs a statement with a fresh key whose certificate the authority
// issues for the given build signer.
func signStatement(authority *fulcio, log rekor, statement []byte, issuer, signerURI, signerDigest string) ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	certificate, err := authority.issue(&key.PublicKey, issuer, signerURI, signerDigest)
	if err != nil {
		return nil, err
	}
	protoBundle, err := sign.Bundle(&sign.DSSEData{Data: statement, PayloadType: format.DSSEPayloadType}, leafKeypair{key: key}, sign.BundleOptions{
		CertificateProvider:        issuedCertificate{certificate: certificate},
		CertificateProviderOptions: &sign.CertificateProviderOptions{IDToken: "fixture"},
		TransparencyLogs:           []sign.Transparency{log},
	})
	if err != nil {
		return nil, err
	}
	return (&bundle.Bundle{Bundle: protoBundle}).MarshalJSON()
}

// trustedRootJSON is the trusted root of the fixtures: the Fulcio authority and the
// virtual Rekor.
func trustedRootJSON(authority *fulcio, virtual *ca.VirtualSigstore) ([]byte, error) {
	logs := virtual.RekorLogs()
	var tlogs []*prototrustroot.TransparencyLogInstance
	for logID, log := range logs {
		rawID, err := hex.DecodeString(logID)
		if err != nil {
			return nil, err
		}
		publicKey, err := x509.MarshalPKIXPublicKey(log.PublicKey)
		if err != nil {
			return nil, err
		}
		tlogs = append(tlogs, &prototrustroot.TransparencyLogInstance{
			BaseUrl:       log.BaseURL,
			HashAlgorithm: protocommon.HashAlgorithm_SHA2_256,
			PublicKey: &protocommon.PublicKey{
				RawBytes:   publicKey,
				KeyDetails: protocommon.PublicKeyDetails_PKIX_ECDSA_P256_SHA_256,
				ValidFor:   validity(log.ValidityPeriodStart, log.ValidityPeriodEnd),
			},
			LogId: &protocommon.LogId{KeyId: rawID},
		})
	}
	trusted := &prototrustroot.TrustedRoot{
		MediaType: root.TrustedRootMediaType01,
		Tlogs:     tlogs,
		CertificateAuthorities: []*prototrustroot.CertificateAuthority{{
			Subject: &protocommon.DistinguishedName{Organization: "spec-proof fixtures", CommonName: "fulcio"},
			Uri:     "https://fulcio.fixtures.invalid",
			CertChain: &protocommon.X509CertificateChain{Certificates: []*protocommon.X509Certificate{
				{RawBytes: authority.intermediate.Raw}, {RawBytes: authority.root.Raw},
			}},
			ValidFor: validity(time.Now().Add(-time.Hour), time.Now().Add(fixtureValidity)),
		}},
	}
	raw, err := protojson.Marshal(trusted)
	if err != nil {
		return nil, err
	}
	var indented json.RawMessage = raw
	return json.MarshalIndent(indented, "", "  ")
}

func validity(start, end time.Time) *protocommon.TimeRange {
	return &protocommon.TimeRange{Start: timestamppb.New(start), End: timestamppb.New(end)}
}

// writeTUFRepository writes a TUF repository, as a mirror serves it, that holds the
// trusted root as its only target and returns the bytes of its root.
func writeTUFRepository(directory string, trustedRoot []byte) ([]byte, error) {
	expires := time.Now().Add(fixtureValidity)
	roles := repository.New()
	roles.SetTargets("targets", metadata.Targets(expires))
	roles.SetSnapshot(metadata.Snapshot(expires))
	roles.SetTimestamp(metadata.Timestamp(expires))
	roles.SetRoot(metadata.Root(expires))

	keys := map[string]crypto.Signer{}
	for _, name := range []string{metadata.ROOT, metadata.TARGETS, metadata.SNAPSHOT, metadata.TIMESTAMP} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		keys[name] = key
		tufKey, err := metadata.KeyFromPublicKey(key.Public())
		if err != nil {
			return nil, err
		}
		if err := roles.Root().Signed.AddKey(tufKey, name); err != nil {
			return nil, err
		}
	}
	target, err := metadata.TargetFile().FromBytes("trusted_root.json", trustedRoot, "sha256")
	if err != nil {
		return nil, err
	}
	roles.Targets(metadata.TARGETS).Signed.Targets["trusted_root.json"] = target
	roles.Snapshot().Signed.Meta["targets.json"] = metadata.MetaFile(1)
	roles.Timestamp().Signed.Meta["snapshot.json"] = metadata.MetaFile(1)

	signer := func(role string) (signature.Signer, error) { return signature.LoadSigner(keys[role], crypto.SHA256) }
	for role, sign := range map[string]func(signature.Signer) error{
		metadata.TARGETS:   func(s signature.Signer) error { _, err := roles.Targets(metadata.TARGETS).Sign(s); return err },
		metadata.SNAPSHOT:  func(s signature.Signer) error { _, err := roles.Snapshot().Sign(s); return err },
		metadata.TIMESTAMP: func(s signature.Signer) error { _, err := roles.Timestamp().Sign(s); return err },
		metadata.ROOT:      func(s signature.Signer) error { _, err := roles.Root().Sign(s); return err },
	} {
		loaded, err := signer(role)
		if err != nil {
			return nil, err
		}
		if err := sign(loaded); err != nil {
			return nil, err
		}
	}
	digest := hex.EncodeToString(target.Hashes["sha256"])
	files := map[string]func(string) error{
		"1.root.json":     func(path string) error { return roles.Root().ToFile(path, true) },
		"1.targets.json":  func(path string) error { return roles.Targets(metadata.TARGETS).ToFile(path, true) },
		"1.snapshot.json": func(path string) error { return roles.Snapshot().ToFile(path, true) },
		"timestamp.json":  func(path string) error { return roles.Timestamp().ToFile(path, true) },
		"targets/" + digest + ".trusted_root.json": func(path string) error { return os.WriteFile(path, trustedRoot, 0o644) },
	}
	for name, write := range files {
		path := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := write(path); err != nil {
			return nil, err
		}
	}
	return os.ReadFile(filepath.Join(directory, "1.root.json"))
}

// generateFixtures rewrites testdata: a TUF repository over a trusted root, and
// bundles of the golden statements signed by the trusted build signer and by
// signers the policy must refuse.
func generateFixtures(statements []testkit.Statement) error {
	virtual, err := ca.NewVirtualSigstore()
	if err != nil {
		return err
	}
	trusted, err := newFulcio()
	if err != nil {
		return err
	}
	untrusted, err := newFulcio()
	if err != nil {
		return err
	}
	rootJSON, err := trustedRootJSON(trusted, virtual)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(testdataDirectory); err != nil {
		return err
	}
	tufRoot, err := writeTUFRepository(filepath.Join(testdataDirectory, snapshotDirectory), rootJSON)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(testdataDirectory, tufRootFile), tufRoot, 0o644); err != nil {
		return err
	}
	log := rekor{virtual: virtual}
	variants := []struct {
		name            string
		authority       *fulcio
		issuer, uri, id string
		only            int
	}{
		{testkit.VariantTrusted, trusted, testkit.FixtureIssuer, testkit.FixtureSignerURI, testkit.FixtureSignerDigest, len(statements)},
		{testkit.VariantOtherDigest, trusted, testkit.FixtureIssuer, testkit.FixtureSignerURI, testkit.OtherSignerDigest, 1},
		{testkit.VariantOtherURI, trusted, testkit.FixtureIssuer, testkit.OtherSignerURI, testkit.FixtureSignerDigest, 1},
		{testkit.VariantOtherIssuer, trusted, testkit.OtherIssuer, testkit.FixtureSignerURI, testkit.FixtureSignerDigest, 1},
		{testkit.VariantOtherCA, untrusted, testkit.FixtureIssuer, testkit.FixtureSignerURI, testkit.FixtureSignerDigest, 1},
	}
	for _, variant := range variants {
		for _, statement := range statements[:variant.only] {
			bundleJSON, err := signStatement(variant.authority, log, statement.Bytes, variant.issuer, variant.uri, variant.id)
			if err != nil {
				return fmt.Errorf("%s: %w", variant.name, err)
			}
			path := filepath.Join(testdataDirectory, bundlesDirectory, variant.name, filepath.FromSlash(statement.Location.BundlePath()))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, bundleJSON, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}
