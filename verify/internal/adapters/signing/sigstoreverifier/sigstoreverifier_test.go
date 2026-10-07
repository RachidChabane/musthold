package sigstoreverifier_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/theupdateframework/go-tuf/v2/metadata"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/verify/internal/adapters/signing/sigstoreverifier"
	"github.com/RachidChabane/spec-proof/verify/internal/domain/refusal"
	"github.com/RachidChabane/spec-proof/verify/internal/domain/trustpolicy"
	"github.com/RachidChabane/spec-proof/verify/internal/testkit"
)

const tamperStride = 11

const (
	verifiesBundle = " [verifies SP-163~1]"
	verifiesRoot   = " [verifies SP-174~1]"
	verifiesEmbed  = " [verifies SP-175~1]"
)

func readTestdata(t testing.TB, elements ...string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(append([]string{testdataDirectory}, elements...)...))
	if err != nil {
		t.Fatalf("%v (regenerate with -update-fixtures)", err)
	}
	return content
}

func loadTrustedRoot(t testing.TB) sigstoreverifier.TrustedRoot {
	t.Helper()
	source := sigstoreverifier.FromSnapshot(readTestdata(t, tufRootFile), filepath.Join(testdataDirectory, snapshotDirectory))
	trusted, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return trusted
}

func pinnedSigner(issuer, uri, digest string) trustpolicy.Sigstore {
	return trustpolicy.Sigstore{Issuer: issuer, Signers: []trustpolicy.BuildSigner{{URI: uri, Digest: digest}}}
}

func newVerifier(t testing.TB, pinned trustpolicy.Sigstore) *sigstoreverifier.Verifier {
	t.Helper()
	verifier, err := sigstoreverifier.New(loadTrustedRoot(t).Material, pinned)
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}

func trustedPin() trustpolicy.Sigstore {
	return pinnedSigner(testkit.FixtureIssuer, testkit.FixtureSignerURI, testkit.FixtureSignerDigest)
}

// fixtureBundle reads the bundle of the first golden statement of a variant.
func fixtureBundle(t testing.TB, variant string) (bundle []byte, statement testkit.Statement) {
	t.Helper()
	statement = testkit.ValidStatements(t)[0]
	return readTestdata(t, bundlesDirectory, variant, filepath.FromSlash(statement.Location.BundlePath())), statement
}

func TestVerifyBundle(t *testing.T) {
	verifier := newVerifier(t, trustedPin())

	t.Run("a bundle of the pinned build signer verifies and gives the exact statement bytes"+verifiesBundle, func(t *testing.T) {
		for _, statement := range testkit.ValidStatements(t) {
			bundle := readTestdata(t, bundlesDirectory, testkit.VariantTrusted, filepath.FromSlash(statement.Location.BundlePath()))
			got, err := verifier.Verify(bundle)
			if err != nil || !bytes.Equal(got, statement.Bytes) {
				t.Fatalf("%s: statement differs: %v", statement.Location.BundlePath(), err)
			}
		}
	})

	t.Run("a build signer other than the pinned one is refused"+verifiesBundle, func(t *testing.T) {
		for name, variant := range map[string]string{
			"another commit digest": testkit.VariantOtherDigest,
			"another workflow URI":  testkit.VariantOtherURI,
			"another issuer":        testkit.VariantOtherIssuer,
		} {
			bundle, _ := fixtureBundle(t, variant)
			if _, err := verifier.Verify(bundle); !refusal.Is(err) {
				t.Errorf("%s: error %v", name, err)
			}
		}
	})

	t.Run("a certificate issued by an authority outside the trusted root is refused"+verifiesBundle, func(t *testing.T) {
		bundle, _ := fixtureBundle(t, testkit.VariantOtherCA)
		if _, err := verifier.Verify(bundle); !refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a policy that pins the signer of a refused bundle accepts it"+verifiesBundle, func(t *testing.T) {
		pinned := newVerifier(t, pinnedSigner(testkit.FixtureIssuer, testkit.FixtureSignerURI, testkit.OtherSignerDigest))
		bundle, statement := fixtureBundle(t, testkit.VariantOtherDigest)
		got, err := pinned.Verify(bundle)
		if err != nil || !bytes.Equal(got, statement.Bytes) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a policy with several signers accepts a bundle of any of them"+verifiesBundle, func(t *testing.T) {
		pinned := trustpolicy.Sigstore{Issuer: testkit.FixtureIssuer, Signers: []trustpolicy.BuildSigner{
			{URI: testkit.OtherSignerURI, Digest: testkit.OtherSignerDigest}, {URI: testkit.FixtureSignerURI, Digest: testkit.FixtureSignerDigest},
		}}
		bundle, _ := fixtureBundle(t, testkit.VariantTrusted)
		if _, err := newVerifier(t, pinned).Verify(bundle); err != nil {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a byte changed anywhere in a bundle is refused"+verifiesBundle, func(t *testing.T) {
		bundle, statement := fixtureBundle(t, testkit.VariantTrusted)
		accepted := 0
		for position := 0; position < len(bundle); position += tamperStride {
			tampered := bytes.Clone(bundle)
			tampered[position] ^= 0x01
			if got, err := verifier.Verify(tampered); err == nil {
				// A change that leaves the verified content intact, such as the case of
				// a base64 digit that decodes to the same bytes, must not change it.
				if !bytes.Equal(got, statement.Bytes) {
					t.Fatalf("byte %d changed the verified statement", position)
				}
				accepted++
			}
		}
		if accepted > len(bundle)/tamperStride/10 {
			t.Fatalf("%d single-byte changes still verified", accepted)
		}
	})

	t.Run("a payload swapped under the signature is refused"+verifiesBundle, func(t *testing.T) {
		bundle, statement := fixtureBundle(t, testkit.VariantTrusted)
		forged := bytes.Replace(bundle, []byte(base64.StdEncoding.EncodeToString(statement.Bytes)), []byte(base64.StdEncoding.EncodeToString(append(bytes.Clone(statement.Bytes[:len(statement.Bytes)-1]), ' ', '}'))), 1)
		if bytes.Equal(forged, bundle) {
			t.Fatal("the payload was not found in the bundle")
		}
		if _, err := verifier.Verify(forged); !refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a bundle that holds a key hint in place of a certificate is refused"+verifiesBundle, func(t *testing.T) {
		bundle, _ := fixtureBundle(t, testkit.VariantTrusted)
		var document map[string]any
		if err := json.Unmarshal(bundle, &document); err != nil {
			t.Fatal(err)
		}
		document["verificationMaterial"] = map[string]any{"publicKey": map[string]any{"hint": "x"}}
		stripped, _ := json.Marshal(document)
		if _, err := verifier.Verify(stripped); !refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a bundle without a transparency log entry is refused"+verifiesBundle, func(t *testing.T) {
		bundle, _ := fixtureBundle(t, testkit.VariantTrusted)
		var document struct {
			MediaType            string                     `json:"mediaType"`
			VerificationMaterial map[string]json.RawMessage `json:"verificationMaterial"`
			DSSE                 json.RawMessage            `json:"dsseEnvelope"`
		}
		if err := json.Unmarshal(bundle, &document); err != nil {
			t.Fatal(err)
		}
		delete(document.VerificationMaterial, "tlogEntries")
		stripped, _ := json.Marshal(document)
		if _, err := verifier.Verify(stripped); !refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a document that is not a bundle is refused"+verifiesBundle, func(t *testing.T) {
		for _, document := range []string{`{`, `{}`, `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json"}`, `[]`} {
			if _, err := verifier.Verify([]byte(document)); !refusal.Is(err) {
				t.Errorf("%s: error %v", document, err)
			}
		}
	})

	t.Run("a verifier that pins no build signer refuses every bundle"+verifiesBundle, func(t *testing.T) {
		unpinned, err := sigstoreverifier.New(loadTrustedRoot(t).Material, trustpolicy.Sigstore{Issuer: testkit.FixtureIssuer})
		if err != nil {
			t.Fatal(err)
		}
		bundle, _ := fixtureBundle(t, testkit.VariantTrusted)
		if _, err := unpinned.Verify(bundle); !refusal.Is(err) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestTrustedRoot(t *testing.T) {
	snapshot := filepath.Join(testdataDirectory, snapshotDirectory)
	tufRoot := readTestdata(t, tufRootFile)

	t.Run("the trusted root is verified through TUF from the exported snapshot and its digest is stated"+verifiesRoot, func(t *testing.T) {
		trusted, err := sigstoreverifier.FromSnapshot(tufRoot, snapshot).Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(filepath.Join(snapshot, "targets"))
		if err != nil || len(entries) != 1 {
			t.Fatalf("targets %v: %v", entries, err)
		}
		raw, err := os.ReadFile(filepath.Join(snapshot, "targets", entries[0].Name()))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if trusted.Digest != hex.EncodeToString(sum[:]) || trusted.Digest != format.SHA256Hex(raw) || len(trusted.Material.FulcioCertificateAuthorities()) != 1 {
			t.Fatalf("digest %s", trusted.Digest)
		}
		if !strings.HasPrefix(entries[0].Name(), trusted.Digest) {
			t.Fatalf("target %s is not named by the digest %s", entries[0].Name(), trusted.Digest)
		}
	})

	t.Run("loading twice gives the same digest and writes nothing outside the snapshot"+verifiesRoot, func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		first, err := sigstoreverifier.FromSnapshot(tufRoot, snapshot).Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		second, err := sigstoreverifier.FromSnapshot(tufRoot, snapshot).Load(context.Background())
		if err != nil || first.Digest != second.Digest {
			t.Fatalf("digests %s %s: %v", first.Digest, second.Digest, err)
		}
		if entries, _ := os.ReadDir(home); len(entries) != 0 {
			t.Fatalf("%d entries written to the home directory", len(entries))
		}
	})

	copySnapshot := func(t *testing.T) string {
		t.Helper()
		directory := t.TempDir()
		err := filepath.WalkDir(snapshot, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			relative, _ := filepath.Rel(snapshot, path)
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			testkit.WriteFile(t, filepath.Join(directory, relative), content)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return directory
	}

	corrupt := map[string]func(t *testing.T, directory string){
		"a missing timestamp": func(t *testing.T, directory string) { mustRemove(t, filepath.Join(directory, "timestamp.json")) },
		"a missing snapshot":  func(t *testing.T, directory string) { mustRemove(t, filepath.Join(directory, "1.snapshot.json")) },
		"a missing targets":   func(t *testing.T, directory string) { mustRemove(t, filepath.Join(directory, "1.targets.json")) },
		"a missing target": func(t *testing.T, directory string) {
			matches, _ := filepath.Glob(filepath.Join(directory, "targets", "*"))
			mustRemove(t, matches[0])
		},
		"a tampered target": func(t *testing.T, directory string) {
			matches, _ := filepath.Glob(filepath.Join(directory, "targets", "*"))
			content, _ := os.ReadFile(matches[0])
			content[len(content)/2] ^= 0x01
			testkit.WriteFile(t, matches[0], content)
		},
		"a tampered targets metadata": func(t *testing.T, directory string) {
			path := filepath.Join(directory, "1.targets.json")
			content, _ := os.ReadFile(path)
			testkit.WriteFile(t, path, bytes.Replace(content, []byte(`"length"`), []byte(`"lengtH"`), 1))
		},
		"a timestamp without a valid signature": func(t *testing.T, directory string) {
			path := filepath.Join(directory, "timestamp.json")
			var document map[string]json.RawMessage
			content, _ := os.ReadFile(path)
			_ = json.Unmarshal(content, &document)
			document["signatures"] = json.RawMessage(`[]`)
			rewritten, _ := json.Marshal(document)
			testkit.WriteFile(t, path, rewritten)
		},
	}
	for name, damage := range corrupt {
		t.Run(name+" is refused"+verifiesRoot, func(t *testing.T) {
			directory := copySnapshot(t)
			damage(t, directory)
			if _, err := sigstoreverifier.FromSnapshot(tufRoot, directory).Load(context.Background()); err == nil {
				t.Fatal("the trusted root loaded")
			}
		})
	}

	t.Run("a snapshot that does not chain to the root of the binary is refused"+verifiesRoot, func(t *testing.T) {
		if _, err := sigstoreverifier.FromSnapshot(sigstoreverifier.PublicGoodRoot(), snapshot).Load(context.Background()); err == nil {
			t.Fatal("a snapshot signed by unknown keys loaded")
		}
	})

	t.Run("an empty snapshot directory is refused"+verifiesRoot, func(t *testing.T) {
		if _, err := sigstoreverifier.FromSnapshot(tufRoot, t.TempDir()).Load(context.Background()); err == nil {
			t.Fatal("an empty snapshot loaded")
		}
	})

	t.Run("the repository can be refreshed from a mirror over HTTP"+verifiesRoot, func(t *testing.T) {
		server := httptest.NewServer(http.FileServer(http.Dir(snapshot)))
		defer server.Close()
		trusted, err := sigstoreverifier.Source{Root: tufRoot, BaseURL: server.URL}.Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		offline := loadTrustedRoot(t)
		if trusted.Digest != offline.Digest {
			t.Fatalf("mirror digest %s, snapshot digest %s", trusted.Digest, offline.Digest)
		}
	})

	t.Run("a mirror that serves a tampered target is refused"+verifiesRoot, func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			content, err := os.ReadFile(filepath.Join(snapshot, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/"))))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if strings.Contains(r.URL.Path, "trusted_root.json") {
				content = append(content, ' ')
			}
			_, _ = w.Write(content)
		}))
		defer server.Close()
		if _, err := (sigstoreverifier.Source{Root: tufRoot, BaseURL: server.URL}).Load(context.Background()); err == nil {
			t.Fatal("a tampered target loaded")
		}
	})
}

func mustRemove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotFetcher(t *testing.T) {
	directory := t.TempDir()
	testkit.WriteFile(t, filepath.Join(directory, "nested", "file.json"), []byte("0123456789"))
	fetcher := sigstoreverifier.SnapshotFetcher{Directory: directory, BaseURL: "https://base.invalid/repo"}

	t.Run("a file below the base URL is read"+verifiesRoot, func(t *testing.T) {
		got, err := fetcher.DownloadFile("https://base.invalid/repo/nested/file.json", 100, 0)
		if err != nil || string(got) != "0123456789" {
			t.Fatalf("content %q: %v", got, err)
		}
	})

	t.Run("an absent file answers with the status of a server that lacks it"+verifiesRoot, func(t *testing.T) {
		_, err := fetcher.DownloadFile("https://base.invalid/repo/absent.json", 100, 0)
		var http *metadata.ErrDownloadHTTP
		if !errors.As(err, &http) || http.StatusCode != 404 {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a file longer than the limit is refused"+verifiesRoot, func(t *testing.T) {
		_, err := fetcher.DownloadFile("https://base.invalid/repo/nested/file.json", 5, 0)
		var mismatch *metadata.ErrDownloadLengthMismatch
		if !errors.As(err, &mismatch) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a URL outside the base or leaving the directory is refused"+verifiesRoot, func(t *testing.T) {
		for _, url := range []string{"https://elsewhere.invalid/nested/file.json", "https://base.invalid/repo/../secret", "https://base.invalid/repo/..%2Fsecret"} {
			if _, err := fetcher.DownloadFile(url, 100, 0); err == nil {
				t.Errorf("%s was read", url)
			}
		}
	})
}

func TestEmbeddedRoot(t *testing.T) {
	t.Run("the binary embeds a TUF root of the public-good instance"+verifiesEmbed, func(t *testing.T) {
		embedded := sigstoreverifier.PublicGoodRoot()
		parsed, err := metadata.Root().FromBytes(embedded)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Signed.Type != metadata.ROOT || parsed.Signed.Version < 1 || len(parsed.Signed.Keys) == 0 || len(parsed.Signed.Roles) == 0 {
			t.Fatalf("root %+v", parsed.Signed)
		}
	})

	t.Run("the embedded root is handed out as a copy"+verifiesEmbed, func(t *testing.T) {
		first := sigstoreverifier.PublicGoodRoot()
		first[0] = 'X'
		if second := sigstoreverifier.PublicGoodRoot(); second[0] == 'X' {
			t.Fatal("the embedded root was modified through a returned slice")
		}
	})
}
