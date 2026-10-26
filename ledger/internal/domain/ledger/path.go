package ledger

import (
	"regexp"
	"strings"

	"github.com/RachidChabane/musthold/format"
)

// Category is the kind of content a ledger path holds.
type Category int

// Categories of ledger path.
const (
	CategoryUnknown Category = iota
	CategoryProof
	CategoryMutant
	CategorySeed
	CategoryTrust
)

const (
	pathSeparator = "/"
	seedSuffix    = ".json"
	// ageHeader opens every file encrypted with age in its binary format.
	ageHeader = "age-encryption.org/v1\n"
)

var headSHAPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

var categoryDirectories = map[string]Category{
	format.ProofsDirectory:  CategoryProof,
	format.MutantsDirectory: CategoryMutant,
	format.SeedsDirectory:   CategorySeed,
	format.TrustDirectory:   CategoryTrust,
}

// Classify returns the category of a ledger path, by its first segment.
func Classify(path string) Category {
	directory, _, found := strings.Cut(path, pathSeparator)
	if !found {
		return CategoryUnknown
	}
	return categoryDirectories[directory]
}

// MutantPath is the path of the encrypted mutants of a cache key.
func MutantPath(cacheKey string) string {
	return format.MutantsDirectory + pathSeparator + cacheKey + format.MutantSuffix
}

// ParseMutantPath reads a path as the encrypted mutants of a cache key.
func ParseMutantPath(path string) (cacheKey string, ok bool) {
	rest, found := strings.CutPrefix(path, format.MutantsDirectory+pathSeparator)
	if !found {
		return "", false
	}
	cacheKey, found = strings.CutSuffix(rest, format.MutantSuffix)
	return cacheKey, found && format.IsSHA256Hex(cacheKey)
}

// IsAgeCiphertext reports whether content is an age file: it carries the header
// of the binary format. A plain-text mutant never enters the ledger.
func IsAgeCiphertext(content []byte) bool {
	return strings.HasPrefix(string(content), ageHeader)
}

// SeedPath is the path of the seed recorded for a head commit.
func SeedPath(headSHA string) string {
	return format.SeedsDirectory + pathSeparator + headSHA + seedSuffix
}

// ParseSeedPath reads a path as the seed of a head commit.
func ParseSeedPath(path string) (headSHA string, ok bool) {
	rest, found := strings.CutPrefix(path, format.SeedsDirectory+pathSeparator)
	if !found {
		return "", false
	}
	headSHA, found = strings.CutSuffix(rest, seedSuffix)
	return headSHA, found && headSHAPattern.MatchString(headSHA)
}

// TrustPath is the path of a file of the trusted-root snapshot with the given id.
func TrustPath(snapshotID, relative string) string {
	return format.TrustDirectory + pathSeparator + snapshotID + pathSeparator + relative
}

// ParseTrustPath reads a path as a file of a trust snapshot: the snapshot id and the
// path of the file below the snapshot.
func ParseTrustPath(path string) (snapshotID, relative string, ok bool) {
	rest, found := strings.CutPrefix(path, format.TrustDirectory+pathSeparator)
	if !found {
		return "", "", false
	}
	snapshotID, relative, found = strings.Cut(rest, pathSeparator)
	return snapshotID, relative, found && format.IsSHA256Hex(snapshotID) && IsRelativeFilePath(relative)
}

// IsRelativeFilePath reports whether path is a relative POSIX path of path
// segments, the shape of every file name the ledger accepts below a directory.
func IsRelativeFilePath(path string) bool {
	if path == "" {
		return false
	}
	for _, segment := range strings.Split(path, pathSeparator) {
		if !format.IsPathSegment(segment) {
			return false
		}
	}
	return true
}
