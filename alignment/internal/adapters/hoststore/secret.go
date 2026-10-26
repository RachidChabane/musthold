package hoststore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/RachidChabane/musthold/alignment/internal/platform/atomicfile"
)

// SecretBytes is the entropy of the secret.
const SecretBytes = 32

// Secret implements hostproto.Secrets: the secret is SecretBytes random bytes in
// hex, created on first use under the lock and stable afterwards.
func (s *Store) Secret() ([]byte, error) {
	return withLock(s, func() ([]byte, error) {
		path := s.path(SecretFile)
		secret, err := os.ReadFile(path)
		if err == nil {
			return secret, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("read secret: %w", err)
		}
		return createSecret(path)
	})
}

func createSecret(path string) ([]byte, error) {
	entropy := make([]byte, SecretBytes)
	if _, err := rand.Read(entropy); err != nil {
		return nil, fmt.Errorf("draw secret: %w", err)
	}
	secret := []byte(hex.EncodeToString(entropy))
	if err := atomicfile.Write(path, secret, FileMode); err != nil {
		return nil, err
	}
	return secret, nil
}
