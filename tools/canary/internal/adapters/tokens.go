package adapters

import (
	"crypto/rand"
	"encoding/hex"
)

// tokenBytes is the number of random bytes of a token: enough that no agent can
// guess it.
const tokenBytes = 16

// RandomTokens draws tokens from the operating system's random source.
type RandomTokens struct{}

// New implements app.Tokens.
func (RandomTokens) New() (string, error) {
	buffer := make([]byte, tokenBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
