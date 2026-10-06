package format

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/gowebpki/jcs"
)

// Canonicalize serializes value as JSON and rewrites it in the RFC 8785 (JCS)
// canonical form: UTF-8, sorted keys, no insignificant whitespace.
func Canonicalize(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	return CanonicalizeJSON(raw)
}

// CanonicalizeJSON rewrites an existing JSON document in the JCS canonical form.
func CanonicalizeJSON(document []byte) ([]byte, error) {
	canonical, err := jcs.Transform(document)
	if err != nil {
		return nil, fmt.Errorf("canonicalize: %w", err)
	}
	return canonical, nil
}

// SHA256Hex returns the lowercase hex SHA-256 digest of data.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// IsSHA256Hex reports whether s is a lowercase hex-encoded SHA-256 digest.
func IsSHA256Hex(s string) bool {
	if len(s) != Sha256HexLength {
		return false
	}
	for _, r := range s {
		isDigit := r >= '0' && r <= '9'
		isLowerHex := r >= 'a' && r <= 'f'
		if !isDigit && !isLowerHex {
			return false
		}
	}
	return true
}
