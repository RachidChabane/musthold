package proofset_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/verify/internal/domain/proofset"
)

const verifiesManifest = " [verifies SP-215~1]"

func location(module, subject string, keyByte string) format.ProofLocation {
	return format.ProofLocation{Module: module, SubjectKey: subject, CacheKey: strings.Repeat(keyByte, 64)}
}

func TestManifestContract(t *testing.T) {
	set := proofset.New()
	set.AddSigned(location("m2", "B~1", "b"), []byte("second"), []byte("bundle two"))
	set.AddSigned(location("m1", "A~1", "a"), []byte("first"), []byte("bundle one"))
	set.AddPending(format.Pending{Module: "m2", Pending: []format.PendingEntry{{Criterion: "Z~1", Reason: format.PendingAwaitingHost}}})
	set.AddPending(format.Pending{Module: "m1", Pending: []format.PendingEntry{
		{Criterion: "C~1", Reason: format.PendingAwaitingProof}, {Criterion: "D~2", Reason: format.PendingHostUnresponsive},
	}})
	header := proofset.Header{VerifierVersion: "1.2.3", TrustedRootDigest: strings.Repeat("1", 64), PolicyDigest: strings.Repeat("2", 64)}

	t.Run("the manifest states the members the gate reads, sorted"+verifiesManifest, func(t *testing.T) {
		encoded, err := set.Manifest(header).Bytes()
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			VerifierVersion, Trust, TrustedRootDigest, PolicyDigest string
			Statements                                              []struct{ Path, Digest string }
			Bundles                                                 []struct{ Path, Digest string }
			Pending                                                 []struct{ Module, Criterion, Reason string }
		}
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.VerifierVersion != "1.2.3" || decoded.Trust != "signed" || decoded.TrustedRootDigest != header.TrustedRootDigest || decoded.PolicyDigest != header.PolicyDigest {
			t.Fatalf("header %+v", decoded)
		}
		wantPaths := []string{"m1/A~1/" + strings.Repeat("a", 64) + ".json", "m2/B~1/" + strings.Repeat("b", 64) + ".json"}
		gotPaths := []string{decoded.Statements[0].Path, decoded.Statements[1].Path}
		if !slices.Equal(gotPaths, wantPaths) {
			t.Fatalf("statement paths %v", gotPaths)
		}
		if decoded.Statements[0].Digest != format.SHA256Hex([]byte("first")) || decoded.Statements[1].Digest != format.SHA256Hex([]byte("second")) {
			t.Fatalf("statement digests %+v", decoded.Statements)
		}
		if decoded.Bundles[0].Path != "proofs/"+wantPaths[0][:len(wantPaths[0])-len(".json")]+".dsse.json" || decoded.Bundles[0].Digest != format.SHA256Hex([]byte("bundle one")) {
			t.Fatalf("bundles %+v", decoded.Bundles)
		}
		want := []struct{ Module, Criterion, Reason string }{
			{"m1", "C~1", "awaiting_proof"}, {"m1", "D~2", "host_unresponsive"}, {"m2", "Z~1", "awaiting_host"},
		}
		if !slices.Equal(decoded.Pending, want) {
			t.Fatalf("pending %+v", decoded.Pending)
		}
	})

	t.Run("the manifest is canonical JSON with exactly the members of the contract"+verifiesManifest, func(t *testing.T) {
		encoded, _ := set.Manifest(header).Bytes()
		canonical, err := format.CanonicalizeJSON(encoded)
		if err != nil || string(canonical) != string(encoded) {
			t.Fatalf("not canonical: %v", err)
		}
		var members map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &members); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"verifierVersion", "trust", "trustedRootDigest", "policyDigest", "statements", "pending"} {
			if _, found := members[name]; !found {
				t.Errorf("manifest has no %s", name)
			}
		}
	})

	t.Run("an empty set gives empty lists, never null"+verifiesManifest, func(t *testing.T) {
		encoded, err := proofset.New().Manifest(proofset.Header{VerifierVersion: "v"}).Bytes()
		if err != nil {
			t.Fatal(err)
		}
		want := `{"bundles":[],"pending":[],"statements":[],"trust":"signed","verifierVersion":"v"}`
		if string(encoded) != want {
			t.Fatalf("manifest %s, want %s", encoded, want)
		}
	})

	t.Run("a set is signed until one statement is local"+verifiesManifest, func(t *testing.T) {
		local := proofset.New()
		local.AddSigned(location("m", "A~1", "a"), []byte("s"), []byte("b"))
		if local.Trust() != proofset.TrustSigned {
			t.Fatal("a signed set is not signed")
		}
		local.AddLocal(location("m", "B~1", "b"), []byte("l"))
		if local.Trust() != proofset.TrustLocal {
			t.Fatal("a set with a local statement is not local")
		}
		manifest := local.Manifest(proofset.Header{})
		if len(manifest.Bundles) != 1 || len(manifest.Statements) != 2 {
			t.Fatalf("manifest %+v", manifest)
		}
	})

	t.Run("a signed statement shadows a local one at the same location in either order"+verifiesManifest, func(t *testing.T) {
		place := location("m", "A~1", "a")
		for name, add := range map[string]func(*proofset.Set){
			"local first": func(s *proofset.Set) {
				s.AddLocal(place, []byte("local"))
				s.AddSigned(place, []byte("signed"), []byte("b"))
			},
			"signed first": func(s *proofset.Set) {
				s.AddSigned(place, []byte("signed"), []byte("b"))
				s.AddLocal(place, []byte("local"))
			},
		} {
			set := proofset.New()
			add(set)
			files := set.Files()
			if len(files) != 1 || string(files[0].Content) != "signed" || set.Trust() != proofset.TrustSigned {
				t.Errorf("%s: files %+v trust %s", name, files, set.Trust())
			}
		}
	})

	t.Run("the manifest does not depend on the order statements were added"+verifiesManifest, func(t *testing.T) {
		keys := rapid.SliceOfNDistinct(rapid.StringMatching(`[a-f]{1}`), 1, 6, func(key string) string { return key })
		rapid.Check(t, func(t *rapid.T) {
			drawn := keys.Draw(t, "keys")
			forward, backward := proofset.New(), proofset.New()
			for _, key := range drawn {
				forward.AddSigned(location("m", "S~1", strings.Repeat(key, 1)), []byte(key), []byte("b"+key))
			}
			for _, key := range slices.Backward(drawn) {
				backward.AddSigned(location("m", "S~1", key), []byte(key), []byte("b"+key))
			}
			a, _ := forward.Manifest(proofset.Header{VerifierVersion: "v"}).Bytes()
			b, _ := backward.Manifest(proofset.Header{VerifierVersion: "v"}).Bytes()
			if string(a) != string(b) {
				t.Fatalf("manifests differ:\n%s\n%s", a, b)
			}
		})
	})

	t.Run("the digest of a trusted root that does not exist is the digest of the empty input"+verifiesManifest, func(t *testing.T) {
		if proofset.NoTrustedRootDigest != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
			t.Fatalf("digest %s", proofset.NoTrustedRootDigest)
		}
	})
}

func TestTrustOfAnEmptySet(t *testing.T) {
	t.Run("a run that verified no bundle states trust local even with no statement [verifies SP-1641~1]", func(t *testing.T) {
		manifest := proofset.New().Manifest(proofset.Header{VerifierVersion: "1", NoBundles: true})
		if manifest.Trust != proofset.TrustLocal || manifest.TrustedRootDigest != "" || manifest.PolicyDigest != "" {
			t.Fatalf("manifest %+v", manifest)
		}
	})
	t.Run("a run that verified bundles keeps the trust of its set [verifies SP-1641~1]", func(t *testing.T) {
		manifest := proofset.New().Manifest(proofset.Header{VerifierVersion: "1"})
		if manifest.Trust != proofset.TrustSigned {
			t.Fatalf("manifest %+v", manifest)
		}
	})
}
