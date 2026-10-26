// Package hostbundle defines what travels between a developer's machine and CI:
// the bundle of host responses a push publishes under a git ref, and the bundle
// of requests CI issues for the host to answer. It fixes the file layout of both,
// their codecs and the rules that decide whether CI may use a pushed bundle. It
// knows no I/O.
package hostbundle

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

// Layout of a bundle, relative to its root.
const (
	// ManifestFile names the manifest of a pushed bundle.
	ManifestFile = "manifest.json"
	// ResponsesDirectory holds one file per response.
	ResponsesDirectory = "responses"
	// IssuedManifestFile names the manifest of a CI-issued bundle.
	IssuedManifestFile = "issued.json"
	// RequestsDirectory holds one file per CI-issued request.
	RequestsDirectory = "requests"
	// RefNamespace is the namespace of the refs a push writes.
	RefNamespace = "refs/musthold/host"
	// IssuedRoot is the project directory that holds the requests CI issues, one
	// directory per module below it, so that no module overwrites another's.
	IssuedRoot = ".musthold/ci-issued"
	// IssuedArtifactPrefix starts the name of the artifact CI uploads its issued
	// requests as; the module name ends it.
	IssuedArtifactPrefix = "musthold-host-"

	recordExtension = ".json"
	keySeparator    = "."
	pathSeparator   = "/"
	seedBytes       = 8
	seedSeparator   = "\x00"
)

// Errors of the bundle codecs and rules.
var (
	// ErrBadBundle is returned for a set of files that is not a bundle.
	ErrBadBundle = errors.New("not a host bundle")
	// ErrVersionSkew is returned when a bundle was pushed by another version of the
	// module than the one CI runs, and CI holds requests the bundle does not answer.
	ErrVersionSkew = errors.New("host_version_skew")
)

// File is one file of a bundle.
type File struct {
	Path    string
	Content []byte
}

// Entry names one response a manifest lists.
type Entry struct {
	RequestID hostproto.RequestID `json:"requestId"`
	Vote      int                 `json:"vote"`
	// Marked is set when the journal marks the response as leaked, exposed or
	// conflicting, which the response itself does not carry.
	Marked bool `json:"marked,omitempty"`
}

// Manifest describes a pushed bundle: who pushed it, for which commit, the head of
// the journal it comes from and the responses it holds.
type Manifest struct {
	Module        format.Module `json:"module"`
	HeadSHA       string        `json:"headSha"`
	JournalDigest string        `json:"journalDigest"`
	Responses     []Entry       `json:"responses"`
}

// Bundle is the content of a push: its manifest and the responses it lists.
type Bundle struct {
	Manifest  Manifest
	Responses []hostproto.Response
}

// RefName is the ref a module's push writes for a commit: the namespace, the
// module's short name (`discpower`, `alignment`, `completeness`) and the commit.
func RefName(module, headSHA string) string {
	return strings.Join([]string{RefNamespace, module, headSHA}, pathSeparator)
}

// IssuedArtifact is the name of the artifact a module's CI job uploads its issued
// requests as.
func IssuedArtifact(module string) string { return IssuedArtifactPrefix + module }

func responsePath(key hostproto.Key) string {
	return ResponsesDirectory + pathSeparator + string(key.RequestID) + keySeparator + strconv.Itoa(key.Vote) + recordExtension
}

func compareKeys(a, b hostproto.Key) int {
	return cmp.Or(cmp.Compare(a.RequestID, b.RequestID), cmp.Compare(a.Vote, b.Vote))
}

// NewBundle builds the bundle of a push: the responses sorted by key, listed in
// the manifest with the marks the journal holds against them.
func NewBundle(module format.Module, headSHA, journalDigest string, responses []hostproto.Response, marked map[hostproto.Key]bool) Bundle {
	sorted := slices.Clone(responses)
	slices.SortFunc(sorted, func(a, b hostproto.Response) int { return compareKeys(a.Key(), b.Key()) })
	entries := make([]Entry, len(sorted))
	for index, response := range sorted {
		entries[index] = Entry{RequestID: response.RequestID, Vote: response.Vote, Marked: marked[response.Key()]}
	}
	return Bundle{Manifest: Manifest{Module: module, HeadSHA: headSHA, JournalDigest: journalDigest, Responses: entries}, Responses: sorted}
}

func encodeFile(path string, value any) (File, error) {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return File{}, fmt.Errorf("encode %s: %w", path, err)
	}
	return File{Path: path, Content: append(content, '\n')}, nil
}

// Files renders the bundle: the manifest and one file per response, sorted by path.
func (b Bundle) Files() ([]File, error) {
	manifest, err := encodeFile(ManifestFile, b.Manifest)
	if err != nil {
		return nil, err
	}
	files := []File{manifest}
	for _, response := range b.Responses {
		file, err := encodeFile(responsePath(response.Key()), response)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func decodeJSON(content []byte, into any) error { return json.Unmarshal(content, into) }

// Decode reads a bundle from its files, keyed by path. The manifest and the files
// must list the same responses, and each response must be valid and sit at the
// path its key gives.
func Decode(files map[string][]byte) (Bundle, error) {
	raw, found := files[ManifestFile]
	if !found {
		return Bundle{}, fmt.Errorf("%w: no %s", ErrBadBundle, ManifestFile)
	}
	var bundle Bundle
	if err := decodeJSON(raw, &bundle.Manifest); err != nil {
		return Bundle{}, fmt.Errorf("%w: %s: %v", ErrBadBundle, ManifestFile, err)
	}
	listed := map[string]bool{}
	for _, entry := range bundle.Manifest.Responses {
		listed[responsePath(hostproto.Key{RequestID: entry.RequestID, Vote: entry.Vote})] = true
	}
	for path, content := range files {
		if !strings.HasPrefix(path, ResponsesDirectory+pathSeparator) {
			continue
		}
		var response hostproto.Response
		if err := decodeJSON(content, &response); err != nil {
			return Bundle{}, fmt.Errorf("%w: %s: %v", ErrBadBundle, path, err)
		}
		if err := response.Validate(); err != nil {
			return Bundle{}, fmt.Errorf("%w: %s: %v", ErrBadBundle, path, err)
		}
		if responsePath(response.Key()) != path || !listed[path] {
			return Bundle{}, fmt.Errorf("%w: %s is not a response the manifest lists", ErrBadBundle, path)
		}
		delete(listed, path)
		bundle.Responses = append(bundle.Responses, response)
	}
	if len(listed) > 0 {
		return Bundle{}, fmt.Errorf("%w: the manifest lists %d responses that are not there", ErrBadBundle, len(listed))
	}
	slices.SortFunc(bundle.Responses, func(a, b hostproto.Response) int { return compareKeys(a.Key(), b.Key()) })
	return bundle, nil
}

// Marked reports whether the manifest marks the response of a vote.
func (b Bundle) Marked(key hostproto.Key) bool {
	return slices.ContainsFunc(b.Manifest.Responses, func(entry Entry) bool {
		return entry.RequestID == key.RequestID && entry.Vote == key.Vote && entry.Marked
	})
}

// Lookup returns the response of a vote of a request.
func (b Bundle) Lookup(key hostproto.Key) (hostproto.Response, bool) {
	for _, response := range b.Responses {
		if response.Key() == key {
			return response, true
		}
	}
	return hostproto.Response{}, false
}

// CheckSkew refuses a bundle pushed by another version of the module than the one
// CI runs when CI holds a request the bundle does not answer: the template of the
// request may have changed between the two, and the ids no longer meet. Both
// versions are named.
func CheckSkew(pushed, running format.Module, unanswered int) error {
	if unanswered == 0 || pushed.Version == running.Version {
		return nil
	}
	return fmt.Errorf("%w: the responses were pushed by %s %s and CI runs %s %s, so %d requests found no answer",
		ErrVersionSkew, pushed.Name, pushed.Version, running.Name, running.Version, unanswered)
}

// CISeed is the seed of a CI run: a hash of the head commit and the first CI run
// that saw it, so that a re-run of the same run draws the same mutants.
func CISeed(headSHA, firstRunID string) uint64 {
	digest := sha256.Sum256([]byte(headSHA + seedSeparator + firstRunID))
	return binary.BigEndian.Uint64(digest[:seedBytes])
}

// IssuedEntry describes one request CI issues.
type IssuedEntry struct {
	RequestID hostproto.RequestID `json:"requestId"`
	Votes     int                 `json:"votes"`
	Criteria  []string            `json:"criteria"`
	Purpose   string              `json:"purpose"`
}

// IssuedBundle is what CI uploads when its run leaves requests for the host: the
// requests in full, since the host needs them to recompute their ids, and the
// queue metadata that goes with them.
type IssuedBundle struct {
	Entries  []IssuedEntry
	Requests []hostproto.Request
}

// Files renders the issued bundle, sorted by request id.
func (b IssuedBundle) Files() ([]File, error) {
	entries := slices.Clone(b.Entries)
	slices.SortFunc(entries, func(a, c IssuedEntry) int { return cmp.Compare(a.RequestID, c.RequestID) })
	manifest, err := encodeFile(IssuedManifestFile, entries)
	if err != nil {
		return nil, err
	}
	files := []File{manifest}
	for _, request := range b.Requests {
		id, err := request.ID()
		if err != nil {
			return nil, err
		}
		file, err := encodeFile(RequestsDirectory+pathSeparator+string(id)+recordExtension, request)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	slices.SortFunc(files[1:], func(a, c File) int { return cmp.Compare(a.Path, c.Path) })
	return files, nil
}

// IssuedDirectory is where a module's CI run leaves the requests it issues,
// relative to the project; the CI job uploads it as the artifact IssuedArtifact
// names.
func IssuedDirectory(module string) string { return IssuedRoot + pathSeparator + module }

// IssuedFilesOf returns the files of a module's issued bundle in a downloaded
// artifact: the artifact itself when its root holds the manifest, and otherwise
// the files below the directory of the module, which is how an artifact looks that
// was uploaded from IssuedRoot.
func IssuedFilesOf(files map[string][]byte, module string) map[string][]byte {
	if _, found := files[IssuedManifestFile]; found {
		return files
	}
	prefix := module + pathSeparator
	below := map[string][]byte{}
	for path, content := range files {
		if rest, found := strings.CutPrefix(path, prefix); found {
			below[rest] = content
		}
	}
	if len(below) == 0 {
		return files
	}
	return below
}

// DecodeIssued reads an issued bundle. Every request must hash to the id of its
// file name and be listed, and every listed request must be there: a request the
// host cannot recompute the id of is never answered.
func DecodeIssued(files map[string][]byte) (IssuedBundle, error) {
	raw, found := files[IssuedManifestFile]
	if !found {
		return IssuedBundle{}, fmt.Errorf("%w: no %s", ErrBadBundle, IssuedManifestFile)
	}
	var bundle IssuedBundle
	if err := decodeJSON(raw, &bundle.Entries); err != nil {
		return IssuedBundle{}, fmt.Errorf("%w: %s: %v", ErrBadBundle, IssuedManifestFile, err)
	}
	listed := map[hostproto.RequestID]bool{}
	for _, entry := range bundle.Entries {
		if entry.Votes < 1 {
			return IssuedBundle{}, fmt.Errorf("%w: %s asks for %d votes", ErrBadBundle, entry.RequestID, entry.Votes)
		}
		listed[entry.RequestID] = true
	}
	for path, content := range files {
		if !strings.HasPrefix(path, RequestsDirectory+pathSeparator) {
			continue
		}
		var request hostproto.Request
		if err := decodeJSON(content, &request); err != nil {
			return IssuedBundle{}, fmt.Errorf("%w: %s: %v", ErrBadBundle, path, err)
		}
		if err := request.Validate(); err != nil {
			return IssuedBundle{}, fmt.Errorf("%w: %s: %v", ErrBadBundle, path, err)
		}
		id, err := request.ID()
		if err != nil {
			return IssuedBundle{}, err
		}
		if RequestsDirectory+pathSeparator+string(id)+recordExtension != path || !listed[id] {
			return IssuedBundle{}, fmt.Errorf("%w: %s is not a request the manifest lists", ErrBadBundle, path)
		}
		delete(listed, id)
		bundle.Requests = append(bundle.Requests, request)
	}
	if len(listed) > 0 {
		return IssuedBundle{}, fmt.Errorf("%w: the manifest lists %d requests that are not there", ErrBadBundle, len(listed))
	}
	return bundle, nil
}

// EntryOf returns the entry of a request.
func (b IssuedBundle) EntryOf(id hostproto.RequestID) (IssuedEntry, bool) {
	for _, entry := range b.Entries {
		if entry.RequestID == id {
			return entry, true
		}
	}
	return IssuedEntry{}, false
}
