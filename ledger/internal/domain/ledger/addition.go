package ledger

import (
	"regexp"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/refusal"
)

// ConflictPolicy says what happens when an addition meets a path that exists with
// other bytes.
type ConflictPolicy int

// Conflict policies.
const (
	// PolicyRefuseDifferent refuses an addition whose path holds other bytes. The
	// same bytes are a no-op. Nothing in the ledger is ever rewritten.
	PolicyRefuseDifferent ConflictPolicy = iota
	// PolicyKeepExisting leaves the existing bytes in place, whatever the new
	// ones are. It serves the seed of a head commit, which the first run fixes.
	PolicyKeepExisting
)

// Addition is a file to add to the ledger.
type Addition struct {
	Path    string
	Content []byte
	Policy  ConflictPolicy
}

// PayloadReader returns the statement a signed bundle carries. It reads the
// payload only: verifying the signature is the verifier's job.
type PayloadReader interface {
	Payload(bundle []byte) ([]byte, error)
}

// targetPathPattern matches a file of a consistent TUF snapshot, whose name
// starts with the digest of its content.
var targetPathPattern = regexp.MustCompile(`^` + format.TrustDirectory + `/targets/([0-9a-f]{64})\.[^/]+$`)

// CheckFile checks that a file belongs in the ledger at its path: the content of
// a proof is a bundle of an admitted, CI-produced statement stored under its own
// module, subject key and cache key; a mutant file is age ciphertext named by its
// cache key; a seed is the one its head commit and run give; a file of the trust
// snapshot has a relative path, and a target is named by its digest.
func CheckFile(path string, content []byte, payloads PayloadReader) error {
	switch Classify(path) {
	case CategoryProof:
		return checkProof(path, content, payloads)
	case CategoryMutant:
		return checkMutant(path, content)
	case CategorySeed:
		return checkSeed(path, content)
	case CategoryTrust:
		return checkTrust(path, content)
	default:
		return refusal.Newf("%s is outside the ledger layout", path)
	}
}

func checkProof(path string, bundle []byte, payloads PayloadReader) error {
	location, ok := format.ParseBundlePath(path)
	if !ok {
		return refusal.Newf("%s is not %s/<module>/<subject key>/<cacheKey>%s", path, format.ProofsDirectory, format.BundleSuffix)
	}
	payload, err := payloads.Payload(bundle)
	if err != nil {
		return refusal.Newf("%s: %v", path, err)
	}
	admitted, err := format.Admit(payload)
	if err != nil {
		return refusal.Newf("%s: %v", path, err)
	}
	if producer := admitted.Proof.Common().ProducedBy; producer != format.ProducedByCI {
		return refusal.Newf("%s: produced by %q, only %q statements are signed", path, producer, format.ProducedByCI)
	}
	if err := admitted.CheckLocation(location); err != nil {
		return refusal.Newf("%s: %v", path, err)
	}
	return nil
}

func checkMutant(path string, content []byte) error {
	if _, ok := ParseMutantPath(path); !ok {
		return refusal.Newf("%s is not mutants/<cacheKey>%s", path, format.MutantSuffix)
	}
	if !IsAgeCiphertext(content) {
		return refusal.Newf("%s is not age ciphertext", path)
	}
	return nil
}

func checkSeed(path string, content []byte) error {
	headSHA, ok := ParseSeedPath(path)
	if !ok {
		return refusal.Newf("%s is not seeds/<head sha>%s", path, seedSuffix)
	}
	record, err := ParseSeedRecord(content)
	if err != nil {
		return err
	}
	if record.HeadSHA != headSHA {
		return refusal.Newf("%s records the seed of %s", path, record.HeadSHA)
	}
	return nil
}

func checkTrust(path string, content []byte) error {
	relative := strings.TrimPrefix(path, format.TrustDirectory+pathSeparator)
	if !IsRelativeFilePath(relative) {
		return refusal.Newf("%s is not a relative file path of the trust snapshot", path)
	}
	if match := targetPathPattern.FindStringSubmatch(path); match != nil && format.SHA256Hex(content) != match[1] {
		return refusal.Newf("%s is not named by the digest of its content", path)
	}
	return nil
}
