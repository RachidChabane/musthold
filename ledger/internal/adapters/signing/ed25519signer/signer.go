// Package ed25519signer signs statements with an Ed25519 identity: the private
// network, self-hosted and air-gapped counterpart of keyless Sigstore signing. It
// wraps the exact statement bytes in a DSSE envelope and signs the DSSE
// pre-authentication encoding with the standard library.
//
// The key is read from the environment of the CI job or from a file only the
// owner can read. A key on a developer machine is never an accepted identity:
// the trust policy of the verifier decides which keys count, and it pins keys of
// CI environments restricted to default-branch workflows.
package ed25519signer

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/RachidChabane/musthold/format"
)

const (
	// KeyEnvironmentVariable holds the PKCS#8 PEM private key of the identity.
	KeyEnvironmentVariable = "MUSTHOLD_ED25519_KEY"

	pemTypePrivateKey = "PRIVATE KEY"
	// forbiddenKeyFileBits are the permission bits of group and others: a key
	// file must be readable by its owner only.
	forbiddenKeyFileBits fs.FileMode = 0o077
)

// Errors reported when the key is loaded.
var (
	ErrNoKey             = errors.New("no Ed25519 key: set " + KeyEnvironmentVariable + " or pass a key file")
	ErrAmbiguousKey      = errors.New("both " + KeyEnvironmentVariable + " and a key file are given")
	ErrKeyFilePermission = errors.New("the key file is readable by group or others")
	ErrKeyFormat         = errors.New("the key is not a PKCS#8 PEM Ed25519 private key")
)

// Signer signs statements with one Ed25519 private key.
type Signer struct {
	key ed25519.PrivateKey
}

// New returns a signer for the key.
func New(key ed25519.PrivateKey) (*Signer, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, ErrKeyFormat
	}
	return &Signer{key: key}, nil
}

// Sign returns the canonical bytes of the DSSE envelope that carries the
// statement.
func (s *Signer) Sign(statement []byte) ([]byte, error) {
	if len(statement) == 0 {
		return nil, errors.New("empty statement")
	}
	return format.SignEnvelope(statement, s.key).Marshal()
}

// KeySource says where the private key comes from.
type KeySource struct {
	// File is the path of a PKCS#8 PEM key file, or empty.
	File string
	// LookupEnv reads an environment variable.
	LookupEnv func(string) (string, bool)
}

// Load reads the private key from the environment variable or from the file; it
// refuses to guess when both are set, and refuses a key file that group or others
// can read.
func (k KeySource) Load() (ed25519.PrivateKey, error) {
	fromEnvironment, hasEnvironment := k.LookupEnv(KeyEnvironmentVariable)
	hasEnvironment = hasEnvironment && fromEnvironment != ""
	switch {
	case hasEnvironment && k.File != "":
		return nil, ErrAmbiguousKey
	case hasEnvironment:
		return parseKey([]byte(fromEnvironment))
	case k.File != "":
		return readKeyFile(k.File)
	default:
		return nil, ErrNoKey
	}
}

func readKeyFile(path string) (ed25519.PrivateKey, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("key file: %w", err)
	}
	if info.Mode().Perm()&forbiddenKeyFileBits != 0 {
		return nil, fmt.Errorf("%w: %s has mode %o, want 0600", ErrKeyFilePermission, path, info.Mode().Perm())
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("key file: %w", err)
	}
	return parseKey(content)
}

func parseKey(content []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(content)
	if block == nil || block.Type != pemTypePrivateKey {
		return nil, ErrKeyFormat
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKeyFormat, err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, ErrKeyFormat
	}
	return key, nil
}
