package format

import "strings"

// Layout of the ledger branch and of the directories derived from it. The ledger
// writes it, the verifier reads the exported copy, and both name the same paths.
const (
	// LedgerBranch is the orphan branch that stores every signed proof.
	LedgerBranch = "spec-proof/ledger"

	// ProofsDirectory holds `<module>/<subject key>/<cacheKey>.dsse.json`.
	ProofsDirectory = "proofs"
	// MutantsDirectory holds `<cacheKey>.age`, the encrypted mutants.
	MutantsDirectory = "mutants"
	// SeedsDirectory holds `<head sha>.json`, the seed of a head commit.
	SeedsDirectory = "seeds"
	// TrustDirectory holds the snapshot of the TUF repository of the trusted root.
	TrustDirectory = "trust"

	// BundleSuffix ends the name of a signed proof.
	BundleSuffix = ".dsse.json"
	// StatementSuffix ends the name of a bare statement.
	StatementSuffix = ".json"
	// MutantSuffix ends the name of an encrypted mutant set.
	MutantSuffix = ".age"

	locationSegments = 3
)

// SubjectKey names the directory of a statement: the criterion `ID~REV`, and for
// a completeness proof the spec path with its separators percent-encoded, so that
// the key is always one path segment and distinct subjects keep distinct keys.
func SubjectKey(subjectName string) string {
	return subjectKeyEscaper.Replace(subjectName)
}

var subjectKeyEscaper = strings.NewReplacer("%", "%25", pathSeparator, "%2F", `\`, "%5C")

// IsPathSegment reports whether name can be one directory or file name of a
// layout path: non-empty, no separator, not a dot entry, no control character
// and no backslash.
func IsPathSegment(name string) bool {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, pathSeparator+`\`) {
		return false
	}
	for _, r := range name {
		if r < ' ' || r == 0x7f {
			return false
		}
	}
	return true
}

// ProofLocation identifies one proof by the segments of its path.
type ProofLocation struct {
	Module     string
	SubjectKey string
	CacheKey   string
}

// IsValid reports whether the location is made of path segments and a SHA-256
// cache key.
func (l ProofLocation) IsValid() bool {
	return IsPathSegment(l.Module) && IsPathSegment(l.SubjectKey) && IsSHA256Hex(l.CacheKey)
}

// BundlePath is the path of the signed proof below the ledger root.
func (l ProofLocation) BundlePath() string {
	return strings.Join([]string{ProofsDirectory, l.Module, l.SubjectKey, l.CacheKey + BundleSuffix}, pathSeparator)
}

// StatementPath is the path of the bare statement below a verified or cache
// directory.
func (l ProofLocation) StatementPath() string {
	return strings.Join([]string{l.Module, l.SubjectKey, l.CacheKey + StatementSuffix}, pathSeparator)
}

// ParseBundlePath reads a path below the ledger root as a proof location.
func ParseBundlePath(path string) (ProofLocation, bool) {
	rest, found := strings.CutPrefix(path, ProofsDirectory+pathSeparator)
	if !found {
		return ProofLocation{}, false
	}
	return parseLocation(rest, BundleSuffix)
}

// ParseStatementPath reads a path below a cache or verified directory as a proof
// location.
func ParseStatementPath(path string) (ProofLocation, bool) {
	return parseLocation(path, StatementSuffix)
}

func parseLocation(path, suffix string) (ProofLocation, bool) {
	segments := strings.Split(path, pathSeparator)
	if len(segments) != locationSegments {
		return ProofLocation{}, false
	}
	cacheKey, found := strings.CutSuffix(segments[2], suffix)
	if !found {
		return ProofLocation{}, false
	}
	location := ProofLocation{Module: segments[0], SubjectKey: segments[1], CacheKey: cacheKey}
	return location, location.IsValid()
}
