package hostproto

import (
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
)

// Shape of tickets and nonces.
const (
	// NonceBytes is the entropy of a nonce: 128 bits.
	NonceBytes = 16
	// NonceHexLength is the length of a hex-encoded nonce.
	NonceHexLength = NonceBytes * 2
	// FirstEmission is the emission number of a ticket's first issue.
	FirstEmission = 1
	// ticketSeparator separates the parts of a ticket identifier.
	ticketSeparator = "."
	ticketParts     = 3
	// ticketPattern finds a ticket identifier inside free text.
	ticketPattern = `\b[0-9a-f]{64}\.[0-9]+\.[0-9]+\b`
)

var ticketRegexp = regexp.MustCompile(ticketPattern)

// Errors of the ticket codec.
var (
	// ErrBadTicket is returned for a string that is not a ticket identifier.
	ErrBadTicket = errors.New("not a ticket identifier")
	// ErrNoSingleTicket is returned when a text holds no ticket, or several.
	ErrNoSingleTicket = errors.New("the text does not hold exactly one ticket")
	// ErrBadNonce is returned for a nonce that is not 128 bits of lowercase hex.
	ErrBadNonce = errors.New("not a nonce")
)

// TicketID identifies one issue of one vote of a request. It is what an executor
// is told; the nonce travels separately.
type TicketID struct {
	RequestID RequestID `json:"requestId"`
	Vote      int       `json:"vote"`
	Emission  int       `json:"emission"`
}

// String renders the identifier as `<requestId>.<vote>.<emission>`.
func (t TicketID) String() string {
	return string(t.RequestID) + ticketSeparator + strconv.Itoa(t.Vote) + ticketSeparator + strconv.Itoa(t.Emission)
}

// Key returns the key of the identifier's entry.
func (t TicketID) Key() Key { return Key{RequestID: t.RequestID, Vote: t.Vote} }

// Validate checks the shape of the identifier.
func (t TicketID) Validate() error {
	var sink problemSink
	checkDigestField(&sink, "requestId", string(t.RequestID))
	if t.Vote < 0 {
		sink.add("vote", "%d is negative", t.Vote)
	}
	if t.Emission < FirstEmission {
		sink.add("emission", "%d is below %d", t.Emission, FirstEmission)
	}
	return sink.err()
}

// ParseTicketID reads `<requestId>.<vote>.<emission>`.
func ParseTicketID(text string) (TicketID, error) {
	parts := strings.Split(text, ticketSeparator)
	if len(parts) != ticketParts {
		return TicketID{}, fmt.Errorf("%w: %q", ErrBadTicket, text)
	}
	vote, voteErr := strconv.Atoi(parts[1])
	emission, emissionErr := strconv.Atoi(parts[2])
	id := TicketID{RequestID: RequestID(parts[0]), Vote: vote, Emission: emission}
	if voteErr != nil || emissionErr != nil || id.Validate() != nil || id.String() != text {
		return TicketID{}, fmt.Errorf("%w: %q", ErrBadTicket, text)
	}
	return id, nil
}

// FindTicketID returns the only ticket identifier a text holds.
func FindTicketID(text string) (TicketID, error) {
	found := ticketRegexp.FindAllString(text, -1)
	if len(found) != 1 {
		return TicketID{}, fmt.Errorf("%w: found %d", ErrNoSingleTicket, len(found))
	}
	return ParseTicketID(found[0])
}

// Ticket is a ticket identifier with the nonce that binds a response to this
// issue. The nonce stays outside the request id.
type Ticket struct {
	TicketID
	Nonce string `json:"nonce"`
}

// NewNonce reads a fresh nonce from random, which adapters wire to a
// cryptographic source.
func NewNonce(random io.Reader) (string, error) {
	raw := make([]byte, NonceBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", fmt.Errorf("read nonce entropy: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// NewTicket issues a ticket for the identifier with a fresh nonce.
func NewTicket(id TicketID, random io.Reader) (Ticket, error) {
	if err := id.Validate(); err != nil {
		return Ticket{}, err
	}
	nonce, err := NewNonce(random)
	if err != nil {
		return Ticket{}, err
	}
	return Ticket{TicketID: id, Nonce: nonce}, nil
}

// ValidNonce reports whether text is 128 bits of lowercase hex.
func ValidNonce(text string) bool {
	if len(text) != NonceHexLength {
		return false
	}
	_, err := hex.DecodeString(text)
	return err == nil && text == strings.ToLower(text)
}

// sameSecret compares two strings in constant time.
func sameSecret(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// OutputDigest is the digest of the canonical form of an output, the identity of
// an answer for the resubmission rule.
func OutputDigest(output []byte) (string, error) {
	canonical, err := format.CanonicalizeJSON(output)
	if err != nil {
		return "", err
	}
	return format.SHA256Hex(canonical), nil
}
