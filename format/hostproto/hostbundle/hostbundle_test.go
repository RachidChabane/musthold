package hostbundle_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbundle"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const headSHA = "0123456789abcdef0123456789abcdef01234567"

var module = format.Module{Name: "spec-proof-dp", Version: "0.1.0"}

func response(request hostproto.Request, vote int) hostproto.Response {
	ticket := hostprototest.Ticket(request, vote, 1)
	return hostprototest.Response(ticket)
}

func filesOf(t *testing.T, bundle hostbundle.Bundle) map[string][]byte {
	t.Helper()
	files, err := bundle.Files()
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string][]byte{}
	for _, file := range files {
		byPath[file.Path] = file.Content
	}
	return byPath
}

func TestBundle(t *testing.T) {
	judge := hostprototest.Request()
	closed := hostprototest.ClosedRequest()
	marked := map[hostproto.Key]bool{response(closed, 0).Key(): true}
	bundle := hostbundle.NewBundle(module, headSHA, hostproto.GenesisDigest, []hostproto.Response{response(closed, 0), response(judge, 1), response(judge, 0)}, marked)

	t.Run("a bundle is its manifest and one file per response, in key order, and decodes back [verifies SP-713~1]", func(t *testing.T) {
		files, err := bundle.Files()
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 4 || files[0].Path != "manifest.json" {
			t.Fatalf("files = %+v", files)
		}
		decoded, err := hostbundle.Decode(filesOf(t, bundle))
		if err != nil {
			t.Fatal(err)
		}
		if len(decoded.Responses) != 3 || decoded.Manifest.HeadSHA != headSHA || decoded.Manifest.Module != module ||
			decoded.Manifest.JournalDigest != hostproto.GenesisDigest {
			t.Fatalf("decoded = %+v", decoded.Manifest)
		}
		for index := 1; index < len(decoded.Manifest.Responses); index++ {
			previous, current := decoded.Manifest.Responses[index-1], decoded.Manifest.Responses[index]
			if previous.RequestID > current.RequestID || (previous.RequestID == current.RequestID && previous.Vote >= current.Vote) {
				t.Errorf("manifest entries %d and %d are out of order", index-1, index)
			}
		}
	})

	t.Run("the same responses in another order give the same bytes [verifies SP-713~1]", func(t *testing.T) {
		other := hostbundle.NewBundle(module, headSHA, hostproto.GenesisDigest, []hostproto.Response{response(judge, 0), response(judge, 1), response(closed, 0)}, marked)
		first, second := filesOf(t, bundle), filesOf(t, other)
		for path, content := range first {
			if string(second[path]) != string(content) {
				t.Errorf("%s differs", path)
			}
		}
	})

	t.Run("the marks the journal holds travel with the manifest [verifies SP-713~1]", func(t *testing.T) {
		decoded, err := hostbundle.Decode(filesOf(t, bundle))
		if err != nil {
			t.Fatal(err)
		}
		if !decoded.Marked(response(closed, 0).Key()) || decoded.Marked(response(judge, 0).Key()) {
			t.Fatalf("manifest = %+v", decoded.Manifest.Responses)
		}
		got, found := decoded.Lookup(response(judge, 1).Key())
		if canonical, _ := format.CanonicalizeJSON(got.Output); !found || string(canonical) != hostprototest.HonestAnswer {
			t.Fatalf("lookup = %+v, %v", got, found)
		}
		if _, found := decoded.Lookup(hostproto.Key{RequestID: "x"}); found {
			t.Fatal("an unknown key was found")
		}
	})

	t.Run("files that are not a consistent bundle are refused [verifies SP-713~1]", func(t *testing.T) {
		good := filesOf(t, bundle)
		clone := func() map[string][]byte {
			copied := map[string][]byte{}
			for path, content := range good {
				copied[path] = content
			}
			return copied
		}
		var someResponse string
		for path := range good {
			if strings.HasPrefix(path, "responses/") {
				someResponse = path
				break
			}
		}
		cases := map[string]func(map[string][]byte){
			"no manifest":                func(f map[string][]byte) { delete(f, "manifest.json") },
			"a broken manifest":          func(f map[string][]byte) { f["manifest.json"] = []byte("{") },
			"a missing response":         func(f map[string][]byte) { delete(f, someResponse) },
			"a broken response":          func(f map[string][]byte) { f[someResponse] = []byte("[]") },
			"a response at a wrong path": func(f map[string][]byte) { f["responses/"+strings.Repeat("a", 64)+".0.json"] = f[someResponse] },
			"an invalid response": func(f map[string][]byte) {
				var parsed map[string]any
				_ = json.Unmarshal(f[someResponse], &parsed)
				parsed["nonce"] = "short"
				f[someResponse], _ = json.Marshal(parsed)
			},
		}
		for name, damage := range cases {
			files := clone()
			damage(files)
			if _, err := hostbundle.Decode(files); !errors.Is(err, hostbundle.ErrBadBundle) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
	})

	t.Run("an empty bundle is a manifest alone [verifies SP-713~1]", func(t *testing.T) {
		empty := hostbundle.NewBundle(module, headSHA, hostproto.GenesisDigest, nil, nil)
		decoded, err := hostbundle.Decode(filesOf(t, empty))
		if err != nil || len(decoded.Responses) != 0 {
			t.Fatalf("decoded = %+v, %v", decoded, err)
		}
	})
}

const refModule = "sample"

func TestRulesOfTheBundle(t *testing.T) {
	t.Run("the ref of a commit sits under the host namespace of the module [verifies SP-713~1]", func(t *testing.T) {
		if got := hostbundle.RefName(refModule, headSHA); got != "refs/spec-proof/host/"+refModule+"/"+headSHA {
			t.Fatalf("ref = %s", got)
		}
		if got := hostbundle.IssuedArtifact(refModule); got != "spec-proof-host-"+refModule {
			t.Fatalf("artifact = %s", got)
		}
	})

	t.Run("a bundle of another version is refused only when requests found no answer, naming both versions [verifies SP-713~1]", func(t *testing.T) {
		older := format.Module{Name: "spec-proof-dp", Version: "0.0.9"}
		if err := hostbundle.CheckSkew(older, module, 0); err != nil {
			t.Errorf("every request answered: %v", err)
		}
		if err := hostbundle.CheckSkew(module, module, 5); err != nil {
			t.Errorf("same version: %v", err)
		}
		err := hostbundle.CheckSkew(older, module, 2)
		if !errors.Is(err, hostbundle.ErrVersionSkew) || !strings.Contains(err.Error(), "0.0.9") || !strings.Contains(err.Error(), "0.1.0") {
			t.Errorf("skew error = %v", err)
		}
	})

	t.Run("the CI seed follows the commit and the first run, and nothing else [verifies SP-713~1]", func(t *testing.T) {
		seed := hostbundle.CISeed(headSHA, "1001")
		if hostbundle.CISeed(headSHA, "1001") != seed {
			t.Error("the seed changes between calls")
		}
		if hostbundle.CISeed(headSHA, "1002") == seed || hostbundle.CISeed(strings.Repeat("f", 40), "1001") == seed {
			t.Error("the seed ignores its inputs")
		}
		if hostbundle.CISeed("ab", "c") == hostbundle.CISeed("a", "bc") {
			t.Error("the inputs run together")
		}
	})
}

func TestIssuedBundle(t *testing.T) {
	judge, closed := hostprototest.Request(), hostprototest.ClosedRequest()
	judgeID, closedID := hostprototest.MustID(judge), hostprototest.MustID(closed)
	issued := hostbundle.IssuedBundle{
		Entries:  []hostbundle.IssuedEntry{{RequestID: judgeID, Votes: 2, Criteria: []string{"PAR-1~1"}, Purpose: "judge"}, {RequestID: closedID, Votes: 3, Criteria: []string{}}},
		Requests: []hostproto.Request{judge, closed},
	}
	files := func() map[string][]byte {
		listed, err := issued.Files()
		if err != nil {
			t.Fatal(err)
		}
		byPath := map[string][]byte{}
		for _, file := range listed {
			byPath[file.Path] = file.Content
		}
		return byPath
	}

	t.Run("issued requests travel in full with their votes and decode back [verifies SP-713~1]", func(t *testing.T) {
		decoded, err := hostbundle.DecodeIssued(files())
		if err != nil || len(decoded.Requests) != 2 {
			t.Fatalf("decoded = %+v, %v", decoded, err)
		}
		entry, found := decoded.EntryOf(judgeID)
		if !found || entry.Votes != 2 || entry.Purpose != "judge" || len(entry.Criteria) != 1 {
			t.Fatalf("entry = %+v", entry)
		}
		for _, request := range decoded.Requests {
			if id, err := request.ID(); err != nil || (id != judgeID && id != closedID) {
				t.Errorf("request id = %s, %v", id, err)
			}
		}
	})

	t.Run("a request the host cannot recompute the id of is refused [verifies SP-713~1]", func(t *testing.T) {
		damaged := files()
		var path string
		for candidate := range damaged {
			if strings.HasPrefix(candidate, "requests/") {
				path = candidate
				break
			}
		}
		var request map[string]any
		if err := json.Unmarshal(damaged[path], &request); err != nil {
			t.Fatal(err)
		}
		request["cacheKey"] = strings.Repeat("e", 64)
		damaged[path], _ = json.Marshal(request)
		if _, err := hostbundle.DecodeIssued(damaged); !errors.Is(err, hostbundle.ErrBadBundle) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a manifest and a directory that disagree, or a bundle without a manifest, are refused [verifies SP-713~1]", func(t *testing.T) {
		missing := files()
		delete(missing, "requests/"+string(judgeID)+".json")
		unlisted := files()
		unlisted["issued.json"] = []byte(`[{"requestId":"` + string(judgeID) + `","votes":1,"criteria":[],"purpose":""}]`)
		noVotes := files()
		noVotes["issued.json"] = []byte(`[{"requestId":"` + string(judgeID) + `","votes":0,"criteria":[],"purpose":""},{"requestId":"` + string(closedID) + `","votes":1,"criteria":[],"purpose":""}]`)
		none := files()
		delete(none, "issued.json")
		for name, damaged := range map[string]map[string][]byte{"missing request": missing, "unlisted request": unlisted, "no votes": noVotes, "no manifest": none} {
			if _, err := hostbundle.DecodeIssued(damaged); !errors.Is(err, hostbundle.ErrBadBundle) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
	})
}

func TestIssuedLocation(t *testing.T) {
	t.Run("each module leaves its requests in a directory of its own below the issued root [verifies SP-1602~1]", func(t *testing.T) {
		dp, alignment := hostbundle.IssuedDirectory("discpower"), hostbundle.IssuedDirectory("alignment")
		if dp != ".spec-proof/ci-issued/discpower" || alignment != ".spec-proof/ci-issued/alignment" || dp == alignment {
			t.Fatalf("directories %q and %q", dp, alignment)
		}
		if !strings.HasPrefix(dp, hostbundle.IssuedRoot+"/") {
			t.Fatalf("%q is not below %q", dp, hostbundle.IssuedRoot)
		}
	})

	t.Run("an artifact holds the issued bundle at its root or below the directory of its module [verifies SP-1602~1]", func(t *testing.T) {
		manifest, request := []byte("[]"), []byte("{}")
		flat := map[string][]byte{"issued.json": manifest, "requests/a.json": request}
		if got := hostbundle.IssuedFilesOf(flat, "discpower"); len(got) != 2 || string(got["issued.json"]) != "[]" {
			t.Errorf("flat artifact: %v", got)
		}
		nested := map[string][]byte{"discpower/issued.json": manifest, "discpower/requests/a.json": request, "alignment/issued.json": []byte("other")}
		got := hostbundle.IssuedFilesOf(nested, "discpower")
		if len(got) != 2 || string(got["issued.json"]) != "[]" || string(got["requests/a.json"]) != "{}" {
			t.Errorf("nested artifact: %v", got)
		}
		if other := hostbundle.IssuedFilesOf(nested, "alignment"); len(other) != 1 || string(other["issued.json"]) != "other" {
			t.Errorf("another module's directory: %v", other)
		}
		if none := hostbundle.IssuedFilesOf(map[string][]byte{"stray": request}, "discpower"); len(none) != 1 {
			t.Errorf("a stray file: %v", none)
		}
	})
}
