package app

import (
	"context"
	"errors"
	"fmt"
)

// Where the credential of an isolated Claude Code session can come from.
const (
	SourceEnvironment = "environment"
	SourceKeychain    = "keychain"
	SourceAbsent      = "absent"
)

// KeychainService is the name of the keychain item that holds the token. The owner
// stores it there under his macOS login.
const KeychainService = "spec-proof-claude-oauth"

// Placeholders a plan prints instead of the value of the credential.
const (
	placeholderSet      = "<set>"
	placeholderKeychain = "<from keychain>"
)

// ErrCredentialAbsent is what a source answers when it holds no credential.
var ErrCredentialAbsent = errors.New("the credential source holds no token")

// ClaudeCredential is one source of the long-lived Claude Code token. The value is
// read only by Token, only when a session is about to start, and never appears in
// an error, a log or a record.
type ClaudeCredential interface {
	// Source names the source: SourceEnvironment or SourceKeychain.
	Source() string
	// Present says whether the source holds a token, without reading its value.
	Present(ctx context.Context) (bool, error)
	// Token returns the value, or ErrCredentialAbsent.
	Token(ctx context.Context) (string, error)
}

// tokenPlaceholder is what a plan prints for the value of the token of a source.
func tokenPlaceholder(source string) string {
	if source == SourceKeychain {
		return placeholderKeychain
	}
	return placeholderSet
}

// firstCredential returns the first source, in order, that holds a token.
func firstCredential(ctx context.Context, sources []ClaudeCredential) (ClaudeCredential, error) {
	for _, source := range sources {
		present, err := source.Present(ctx)
		if err != nil {
			return nil, fmt.Errorf("the %s credential source could not be checked: %w", source.Source(), err)
		}
		if present {
			return source, nil
		}
	}
	return nil, nil
}
