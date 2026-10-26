// Package proofset is the domain model of the verified set the verifier writes: the
// bare statements it admitted and the manifest that Shallnot's gate reads. The
// manifest is the contract of the gate: its member names, its sort orders and its
// digests are fixed by it.
package proofset

import (
	"cmp"
	"slices"

	"github.com/RachidChabane/musthold/format"
)

// Trust says what stands behind the statements of a verified set.
type Trust string

// Trust levels of the manifest.
const (
	// TrustSigned: every statement carries a signature that the trust policy accepts.
	TrustSigned Trust = "signed"
	// TrustLocal: at least one statement was admitted without a signature, from a
	// local cache.
	TrustLocal Trust = "local"
)

// ManifestFileName is the name of the manifest in the verified directory.
const ManifestFileName = "manifest.json"

// NoTrustedRootDigest stands for the trusted root of a policy that trusts no
// Sigstore signer: the digest of the empty input.
var NoTrustedRootDigest = format.SHA256Hex(nil)

// Origin tells how a statement entered the set.
type Origin int

// Origins of a statement.
const (
	OriginSigned Origin = iota
	OriginLocal
)

// entry is one admitted statement.
type entry struct {
	location format.ProofLocation
	origin   Origin
	content  []byte
	// bundle is the signed bundle of a signed statement.
	bundle []byte
}

// PendingItem is one criterion a module still waits on, as the manifest lists it.
type PendingItem struct {
	Module    string               `json:"module"`
	Criterion string               `json:"criterion"`
	Reason    format.PendingReason `json:"reason"`
}

// FileDigest names a file and the SHA-256 of its exact bytes.
type FileDigest struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// Manifest is `manifest.json` of the verified directory, the input contract of the
// gate: the verifier version, the trust level, the digests of the trusted root and
// of the policy (present when signed), the statements and the bundles they came
// from sorted by path, and the pending criteria sorted by module, criterion and
// reason.
type Manifest struct {
	VerifierVersion   string        `json:"verifierVersion"`
	Trust             Trust         `json:"trust"`
	TrustedRootDigest string        `json:"trustedRootDigest,omitempty"`
	PolicyDigest      string        `json:"policyDigest,omitempty"`
	Statements        []FileDigest  `json:"statements"`
	Bundles           []FileDigest  `json:"bundles"`
	Pending           []PendingItem `json:"pending"`
}

// Bytes returns the manifest in RFC 8785 canonical form, the exact content of the
// file.
func (m Manifest) Bytes() ([]byte, error) {
	return format.Canonicalize(m)
}

// File is a file of the verified directory.
type File struct {
	Path    string
	Content []byte
}

// Set accumulates the admitted statements and the pending criteria.
type Set struct {
	entries map[string]entry
	pending []PendingItem
}

// New returns an empty set.
func New() *Set {
	return &Set{entries: map[string]entry{}}
}

// AddSigned admits a statement whose signature was verified, with the bundle it came
// from. A signed statement replaces a local one at the same location: the cache
// key does not cover the producer, so both can exist for one criterion.
func (s *Set) AddSigned(location format.ProofLocation, statement, bundle []byte) {
	s.entries[location.StatementPath()] = entry{location: location, origin: OriginSigned, content: statement, bundle: bundle}
}

// AddLocal admits an unsigned statement of the local cache unless a signed one
// already holds its location.
func (s *Set) AddLocal(location format.ProofLocation, statement []byte) {
	path := location.StatementPath()
	if existing, found := s.entries[path]; found && existing.origin == OriginSigned {
		return
	}
	s.entries[path] = entry{location: location, origin: OriginLocal, content: statement}
}

// AddPending records the criteria a module waits on.
func (s *Set) AddPending(pending format.Pending) {
	for _, item := range pending.Pending {
		s.pending = append(s.pending, PendingItem{Module: pending.Module, Criterion: item.Criterion, Reason: item.Reason})
	}
}

// Trust returns the trust level of the set: local as soon as one statement is
// unsigned, signed otherwise.
func (s *Set) Trust() Trust {
	for _, entry := range s.entries {
		if entry.origin == OriginLocal {
			return TrustLocal
		}
	}
	return TrustSigned
}

// Files returns the statement files of the set, sorted by path.
func (s *Set) Files() []File {
	files := make([]File, 0, len(s.entries))
	for _, path := range s.sortedPaths() {
		files = append(files, File{Path: path, Content: s.entries[path].content})
	}
	return files
}

func (s *Set) sortedPaths() []string {
	paths := make([]string, 0, len(s.entries))
	for path := range s.entries {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths
}

// Header is what the manifest states besides the content of the set.
type Header struct {
	VerifierVersion   string
	TrustedRootDigest string
	PolicyDigest      string
	// NoBundles is true when the run verified no signed bundle: nothing then stands
	// behind the set but the local cache, whether or not the cache held a statement.
	NoBundles bool
}

// trust is the trust level the manifest states: local when the run verified no
// bundle, else the level of the set.
func (s *Set) trust(header Header) Trust {
	if header.NoBundles {
		return TrustLocal
	}
	return s.Trust()
}

// Manifest builds the manifest of the set. The digests of the trusted root and of
// the policy are only stated when the set is signed or the header carries them.
func (s *Set) Manifest(header Header) Manifest {
	manifest := Manifest{
		VerifierVersion:   header.VerifierVersion,
		Trust:             s.trust(header),
		TrustedRootDigest: header.TrustedRootDigest,
		PolicyDigest:      header.PolicyDigest,
		Statements:        []FileDigest{},
		Bundles:           []FileDigest{},
		Pending:           slices.Clone(s.pending),
	}
	if manifest.Pending == nil {
		manifest.Pending = []PendingItem{}
	}
	for _, path := range s.sortedPaths() {
		entry := s.entries[path]
		manifest.Statements = append(manifest.Statements, FileDigest{Path: path, Digest: format.SHA256Hex(entry.content)})
		if entry.origin == OriginSigned {
			manifest.Bundles = append(manifest.Bundles, FileDigest{Path: entry.location.BundlePath(), Digest: format.SHA256Hex(entry.bundle)})
		}
	}
	slices.SortFunc(manifest.Bundles, func(a, b FileDigest) int { return cmp.Compare(a.Path, b.Path) })
	slices.SortFunc(manifest.Pending, comparePending)
	return manifest
}

func comparePending(a, b PendingItem) int {
	return cmp.Or(cmp.Compare(a.Module, b.Module), cmp.Compare(a.Criterion, b.Criterion), cmp.Compare(a.Reason, b.Reason))
}
