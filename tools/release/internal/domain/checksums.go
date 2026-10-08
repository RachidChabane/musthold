package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// File names of the checksum list and of the signature over it.
const (
	ChecksumsFileName   = "SHA256SUMS"
	SignatureBundleName = ChecksumsFileName + ".sigstore.json"
)

const (
	checksumSeparator = "  "
	lineBreak         = "\n"
)

var (
	digestPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	fileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
)

// Checksum is one line of a SHA256SUMS file.
type Checksum struct {
	// Digest is the lower-case hexadecimal SHA-256 of the file.
	Digest string
	Name   string
}

// DigestOf returns the lower-case hexadecimal SHA-256 of data.
func DigestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// RenderChecksums renders a SHA256SUMS file in the format `sha256sum -c` reads:
// `<hex>  <name>`, one per line, sorted by name.
func RenderChecksums(sums []Checksum) string {
	sorted := slices.Clone(sums)
	slices.SortFunc(sorted, func(a, b Checksum) int { return strings.Compare(a.Name, b.Name) })
	var out strings.Builder
	for _, sum := range sorted {
		out.WriteString(sum.Digest + checksumSeparator + sum.Name + lineBreak)
	}
	return out.String()
}

// ParseChecksums parses a SHA256SUMS file. A malformed digest, a malformed or
// repeated name and a missing final line break are refused.
func ParseChecksums(text string) ([]Checksum, error) {
	if text != "" && !strings.HasSuffix(text, lineBreak) {
		return nil, fmt.Errorf("%s does not end with a line break", ChecksumsFileName)
	}
	var sums []Checksum
	seen := map[string]bool{}
	for number, line := range strings.Split(strings.TrimSuffix(text, lineBreak), lineBreak) {
		if text == "" {
			break
		}
		digest, name, found := strings.Cut(line, checksumSeparator)
		if !found || !digestPattern.MatchString(digest) || !fileNamePattern.MatchString(name) {
			return nil, fmt.Errorf("%s line %d is not `<sha256>  <name>`: %q", ChecksumsFileName, number+1, line)
		}
		if seen[name] {
			return nil, fmt.Errorf("%s names %s twice", ChecksumsFileName, name)
		}
		seen[name] = true
		sums = append(sums, Checksum{Digest: digest, Name: name})
	}
	return sums, nil
}

// ChecksumFor returns the digest a list holds for a file name.
func ChecksumFor(sums []Checksum, name string) (string, bool) {
	for _, sum := range sums {
		if sum.Name == name {
			return sum.Digest, true
		}
	}
	return "", false
}
