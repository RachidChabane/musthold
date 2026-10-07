package app_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/verify/internal/app"
	"github.com/RachidChabane/spec-proof/verify/internal/domain/proofset"
	"github.com/RachidChabane/spec-proof/verify/internal/domain/refusal"
	"github.com/RachidChabane/spec-proof/verify/internal/testkit"
)

const (
	verifiesAdmission = " [verifies SP-164~1]"
	verifiesLocal     = " [verifies SP-165~1]"
	verifiesPending   = " [verifies SP-167~1]"
)

// payloadVerifier treats a bundle as its own statement and refuses the bytes
// "forged".
type payloadVerifier struct{}

func (payloadVerifier) Verify(bundle []byte) ([]byte, error) {
	if string(bundle) == "forged" {
		return nil, refusal.Newf("bad signature")
	}
	return bundle, nil
}

type fixedBundles []app.BundleFile

func (f fixedBundles) Bundles() ([]app.BundleFile, error) { return f, nil }

type fixedLocal []app.LocalStatement

func (f fixedLocal) LocalStatements() ([]app.LocalStatement, error) { return f, nil }

type fixedPending []format.Pending

func (f fixedPending) Pending() ([]format.Pending, error) { return f, nil }

type memorySink struct {
	files    []proofset.File
	manifest []byte
	writes   int
}

func (m *memorySink) Write(files []proofset.File, manifest []byte) error {
	m.files, m.manifest, m.writes = files, manifest, m.writes+1
	return nil
}

func service(sink *memorySink, bundles []app.BundleFile, local []app.LocalStatement) app.VerifyService {
	return app.VerifyService{
		Version: "test", Signed: true, TrustedRootDigest: "root", PolicyDigest: "policy",
		Bundles: fixedBundles(bundles), Local: fixedLocal(local), Pending: fixedPending(nil),
		Verifier: payloadVerifier{}, Sink: sink,
	}
}

func bundlesOf(statements []testkit.Statement) []app.BundleFile {
	var bundles []app.BundleFile
	for _, statement := range statements {
		bundles = append(bundles, app.BundleFile{Location: statement.Location, Bytes: statement.Bytes})
	}
	return bundles
}

func TestRunSigned(t *testing.T) {
	statements := testkit.ValidStatements(t)

	t.Run("verified bundles become statement files and a signed manifest"+verifiesAdmission, func(t *testing.T) {
		sink := &memorySink{}
		report, err := service(sink, bundlesOf(statements), nil).Run()
		if err != nil || report.Trust != proofset.TrustSigned || report.Signed != 3 || report.Statements != 3 {
			t.Fatalf("report %+v: %v", report, err)
		}
		if len(sink.files) != 3 || !bytes.Contains(sink.manifest, []byte(`"trustedRootDigest":"root"`)) || !bytes.Contains(sink.manifest, []byte(`"policyDigest":"policy"`)) {
			t.Fatalf("files %d manifest %s", len(sink.files), sink.manifest)
		}
	})

	rejected := map[string]func([]app.BundleFile) []app.BundleFile{
		"a bundle whose signature does not verify": func(b []app.BundleFile) []app.BundleFile {
			b[0].Bytes = []byte("forged")
			return b
		},
		"a signed statement that is not canonical": func(b []app.BundleFile) []app.BundleFile {
			b[0].Bytes = append(bytes.Clone(b[0].Bytes), '\n')
			return b
		},
		"a signed statement produced locally": func(b []app.BundleFile) []app.BundleFile {
			b[0].Bytes = statements[0].AsLocal(t).Bytes
			return b
		},
		"a statement at the wrong cache key": func(b []app.BundleFile) []app.BundleFile {
			b[0].Location.CacheKey = format.SHA256Hex([]byte("elsewhere"))
			return b
		},
		"a statement under the wrong module's subject key": func(b []app.BundleFile) []app.BundleFile {
			b[0].Location.SubjectKey = "OTHER-1~1"
			return b
		},
		"a statement of a predicate type outside the format": func(b []app.BundleFile) []app.BundleFile {
			b[0].Bytes = bytes.Replace(b[0].Bytes, []byte("/alignment/v1"), []byte("/other/v1"), 1)
			return b
		},
	}
	for name, corrupt := range rejected {
		t.Run(name+" rejects the run and writes nothing"+verifiesAdmission, func(t *testing.T) {
			sink := &memorySink{}
			_, err := service(sink, corrupt(bundlesOf(statements)), nil).Run()
			if !errors.Is(err, refusal.ErrRefused) || sink.writes != 0 {
				t.Fatalf("error %v, writes %d", err, sink.writes)
			}
		})
	}

	t.Run("every rejected bundle is reported"+verifiesAdmission, func(t *testing.T) {
		bundles := bundlesOf(statements)
		bundles[0].Bytes, bundles[1].Bytes = []byte("forged"), []byte("forged")
		_, err := service(&memorySink{}, bundles, nil).Run()
		if err == nil || bytes.Count([]byte(err.Error()), []byte("bad signature")) != 2 {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("no bundle gives an empty signed set"+verifiesAdmission, func(t *testing.T) {
		sink := &memorySink{}
		report, err := service(sink, nil, nil).Run()
		if err != nil || report.Statements != 0 || report.Trust != proofset.TrustSigned || sink.writes != 1 {
			t.Fatalf("report %+v: %v", report, err)
		}
	})
}

func TestRunLocal(t *testing.T) {
	statements := testkit.ValidStatements(t)
	local := func(statements ...testkit.Statement) []app.LocalStatement {
		var listed []app.LocalStatement
		for _, statement := range statements {
			listed = append(listed, app.LocalStatement{Location: statement.Location, Bytes: statement.Bytes})
		}
		return listed
	}

	t.Run("an unsigned statement produced locally is admitted and makes the trust local"+verifiesLocal, func(t *testing.T) {
		sink := &memorySink{}
		unsigned := service(sink, nil, local(statements[0].AsLocal(t)))
		unsigned.Signed, unsigned.Bundles = false, app.NoBundles{}
		report, err := unsigned.Run()
		if err != nil || report.Trust != proofset.TrustLocal || report.Local != 1 {
			t.Fatalf("report %+v: %v", report, err)
		}
		if bytes.Contains(sink.manifest, []byte("policyDigest")) || bytes.Contains(sink.manifest, []byte("trustedRootDigest")) {
			t.Fatalf("a local manifest states digests of trust material: %s", sink.manifest)
		}
	})

	t.Run("an unsigned statement produced by CI is refused"+verifiesLocal, func(t *testing.T) {
		sink := &memorySink{}
		_, err := service(sink, nil, local(statements[0])).Run()
		if !errors.Is(err, refusal.ErrRefused) || sink.writes != 0 {
			t.Fatalf("error %v, writes %d", err, sink.writes)
		}
	})

	t.Run("a local statement at the wrong location is refused"+verifiesLocal, func(t *testing.T) {
		moved := statements[0].AsLocal(t)
		moved.Location.CacheKey = format.SHA256Hex([]byte("elsewhere"))
		if _, err := service(&memorySink{}, nil, local(moved)).Run(); !errors.Is(err, refusal.ErrRefused) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a signed statement shadows the local one of the same criterion and inputs"+verifiesLocal, func(t *testing.T) {
		sink := &memorySink{}
		mixed := service(sink, bundlesOf(statements[:1]), local(statements[0].AsLocal(t), statements[1].AsLocal(t)))
		report, err := mixed.Run()
		if err != nil {
			t.Fatal(err)
		}
		if report.Trust != proofset.TrustLocal || len(sink.files) != 2 {
			t.Fatalf("report %+v files %d", report, len(sink.files))
		}
		for _, file := range sink.files {
			if file.Path == statements[0].Location.StatementPath() && !bytes.Equal(file.Content, statements[0].Bytes) {
				t.Fatal("the local statement replaced the signed one")
			}
		}
	})
}

func TestRunPending(t *testing.T) {
	t.Run("the pending files of every module are folded into the manifest"+verifiesPending, func(t *testing.T) {
		sink := &memorySink{}
		run := service(sink, nil, nil)
		run.Pending = fixedPending{
			{Module: "discpower", Pending: []format.PendingEntry{{Criterion: "PARK-2~1", Reason: format.PendingAwaitingHost}}},
			{Module: "alignment", Pending: []format.PendingEntry{{Criterion: "PARK-1~1", Reason: format.PendingAwaitingProof}}},
		}
		report, err := run.Run()
		if err != nil || report.Pending != 2 {
			t.Fatalf("report %+v: %v", report, err)
		}
		want := `"pending":[{"criterion":"PARK-1~1","module":"alignment","reason":"awaiting_proof"},{"criterion":"PARK-2~1","module":"discpower","reason":"awaiting_host"}]`
		if !bytes.Contains(sink.manifest, []byte(want)) {
			t.Fatalf("manifest %s", sink.manifest)
		}
	})
}

func TestSchemeSwitch(t *testing.T) {
	statement := []byte("statement")
	envelope := mustMarshal(t, format.SignEnvelope(statement, testkit.NewIdentity(1).Private))
	sigstoreBundle := []byte(`{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json"}`)

	t.Run("an envelope goes to the Ed25519 verifier and a bundle to the Sigstore one"+verifiesAdmission, func(t *testing.T) {
		envelopeVerifier, sigstoreVerifier := &countingVerifier{}, &countingVerifier{}
		schemes := app.SchemeSwitch{Envelope: envelopeVerifier, Sigstore: sigstoreVerifier}
		if _, err := schemes.Verify(envelope); err != nil {
			t.Fatal(err)
		}
		if _, err := schemes.Verify(sigstoreBundle); err != nil {
			t.Fatal(err)
		}
		if envelopeVerifier.calls != 1 || sigstoreVerifier.calls != 1 {
			t.Fatalf("calls %d %d", envelopeVerifier.calls, sigstoreVerifier.calls)
		}
	})

	t.Run("a scheme the policy does not pin is refused"+verifiesAdmission, func(t *testing.T) {
		if _, err := (app.SchemeSwitch{}).Verify(envelope); !refusal.Is(err) {
			t.Fatalf("envelope: %v", err)
		}
		if _, err := (app.SchemeSwitch{}).Verify(sigstoreBundle); !errors.Is(err, app.ErrNoSigstorePolicy) {
			t.Fatalf("bundle: %v", err)
		}
	})

	t.Run("a document of neither scheme is refused"+verifiesAdmission, func(t *testing.T) {
		if _, err := (app.SchemeSwitch{}).Verify([]byte(`{"hello":"world"}`)); !errors.Is(err, app.ErrUnknownBundle) {
			t.Fatalf("error %v", err)
		}
	})
}

type countingVerifier struct{ calls int }

func (c *countingVerifier) Verify(bundle []byte) ([]byte, error) {
	c.calls++
	return bundle, nil
}

func mustMarshal(t *testing.T, envelope format.Envelope) []byte {
	t.Helper()
	encoded, err := envelope.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
