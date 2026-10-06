// Package mutantvault encrypts mutant bytes to an age X25519 recipient so that
// only the holder of the matching identity can read them.
package mutantvault

import (
	"bytes"
	"fmt"
	"io"

	"filippo.io/age"
)

// Encrypt encrypts plaintext to the age X25519 recipient (an `age1...` string).
func Encrypt(plaintext []byte, recipient string) ([]byte, error) {
	parsed, err := age.ParseX25519Recipient(recipient)
	if err != nil {
		return nil, fmt.Errorf("parse recipient: %w", err)
	}
	var sealed bytes.Buffer
	writer, err := age.Encrypt(&sealed, parsed)
	if err != nil {
		return nil, fmt.Errorf("start encryption: %w", err)
	}
	if _, err := writer.Write(plaintext); err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish encryption: %w", err)
	}
	return sealed.Bytes(), nil
}

// Decrypt decrypts ciphertext with the age X25519 identity (an `AGE-SECRET-KEY-1...`
// string). Only the audit path and tests use it.
func Decrypt(ciphertext []byte, identity string) ([]byte, error) {
	parsed, err := age.ParseX25519Identity(identity)
	if err != nil {
		return nil, fmt.Errorf("parse identity: %w", err)
	}
	reader, err := age.Decrypt(bytes.NewReader(ciphertext), parsed)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	plaintext, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read plaintext: %w", err)
	}
	return plaintext, nil
}
