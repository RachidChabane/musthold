// Package deviation writes the deviation log of the loop-1 study (PREREG §12) and
// the RFC 3161 timestamps its protocol and token-exposed entries carry. The same
// timestamps freeze the pre-registration, the amendment and the inputs (PREREG
// §13), so the client and the reader of a token live here.
package deviation

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Markers of the text `openssl ts -reply -text` prints.
const (
	statusPrefix    = "Status: "
	statusGranted   = "Granted"
	algorithmPrefix = "Hash Algorithm: "
	algorithmSHA256 = "sha256"
	messageMarker   = "Message data:"
	timePrefix      = "Time stamp: "
	timeLayout      = "Jan _2 15:04:05 2006 MST"
)

// The dump openssl prints for the message data is one row per 16 bytes:
// "    0000 - xx xx xx xx xx xx xx xx-xx xx xx xx xx xx xx xx   <ascii>".
const (
	dumpOffsetWidth = 4
	dumpSeparator   = " - "
	dumpPairStride  = 3
	dumpPairWidth   = 2
	dumpRowWidth    = 47
)

// ErrToken is returned for a reply that openssl does not print as a granted
// SHA-256 time-stamp token.
var ErrToken = errors.New("the time-stamp reply cannot be read")

// Token is what a freeze reads from a time-stamp reply.
type Token struct {
	// Imprint is the message imprint, in lowercase hexadecimal.
	Imprint string
	// GenTime is the time the authority stamped, in UTC.
	GenTime time.Time
}

// ParseToken reads the text of `openssl ts -reply -text`.
func ParseToken(text string) (Token, error) {
	var (
		granted, sha256 bool
		inMessage       bool
		imprint         []byte
		stamped         time.Time
	)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, statusPrefix):
			granted = strings.HasPrefix(strings.TrimPrefix(trimmed, statusPrefix), statusGranted)
		case strings.HasPrefix(trimmed, algorithmPrefix):
			sha256 = strings.TrimPrefix(trimmed, algorithmPrefix) == algorithmSHA256
		case trimmed == messageMarker:
			inMessage = true
		case strings.HasPrefix(trimmed, timePrefix):
			parsed, err := time.Parse(timeLayout, strings.TrimPrefix(trimmed, timePrefix))
			if err != nil {
				return Token{}, fmt.Errorf("%w: generation time: %v", ErrToken, err)
			}
			stamped = parsed.UTC()
			inMessage = false
		case inMessage:
			row, ok := dumpRow(trimmed)
			if !ok {
				inMessage = false
				continue
			}
			imprint = append(imprint, row...)
		}
	}
	switch {
	case !granted:
		return Token{}, fmt.Errorf("%w: the status is not granted", ErrToken)
	case !sha256:
		return Token{}, fmt.Errorf("%w: the hash algorithm is not %s", ErrToken, algorithmSHA256)
	case len(imprint) == 0:
		return Token{}, fmt.Errorf("%w: no message data", ErrToken)
	case stamped.IsZero():
		return Token{}, fmt.Errorf("%w: no generation time", ErrToken)
	}
	return Token{Imprint: hex.EncodeToString(imprint), GenTime: stamped}, nil
}

// dumpRow reads the bytes of one row of the hexadecimal dump, by fixed columns:
// the ASCII column that follows can hold any character.
func dumpRow(line string) ([]byte, bool) {
	offset, rest, found := strings.Cut(line, dumpSeparator)
	if !found || len(offset) != dumpOffsetWidth {
		return nil, false
	}
	if _, err := hex.DecodeString(offset); err != nil {
		return nil, false
	}
	var row []byte
	for start := 0; start+dumpPairWidth <= len(rest) && start < dumpRowWidth; start += dumpPairStride {
		value, err := hex.DecodeString(rest[start : start+dumpPairWidth])
		if err != nil {
			break
		}
		row = append(row, value...)
	}
	return row, len(row) > 0
}
