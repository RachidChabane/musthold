package runs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
)

// Separators of the file list an equipped tree's digest is taken over.
const (
	digestPathEnd  = "\x00"
	digestEntryEnd = "\n"
)

// EquippedDigest is the digest of an equipped tree (PREREG §3.5): each file's
// content has every occurrence of the run directory written as `<run>`; the
// digest is the SHA-256 of the list, sorted by path, of each path, a NUL, the
// SHA-256 of its normalised content in hexadecimal, and a line break.
func EquippedDigest(files map[string][]byte, runDir string) string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	var list strings.Builder
	for _, path := range paths {
		list.WriteString(path + digestPathEnd + SHA256(Normalised(files[path], runDir)) + digestEntryEnd)
	}
	return SHA256([]byte(list.String()))
}

// Normalised is a file's content with every occurrence of the run directory
// written as `<run>`.
func Normalised(content []byte, runDir string) []byte {
	return bytes.ReplaceAll(content, []byte(runDir), []byte(app.PlaceholderRun))
}

// SHA256 is the SHA-256 of the content, in hexadecimal.
func SHA256(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// TrackedFiles splits the NUL-separated output of `git ls-files -z`.
func TrackedFiles(output string) []string {
	var paths []string
	for _, path := range strings.Split(output, digestPathEnd) {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}
