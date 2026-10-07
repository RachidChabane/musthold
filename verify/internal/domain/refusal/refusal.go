// Package refusal marks the errors that are a verdict of the verifier, as opposed
// to a failure of the tool: a bundle whose signature the policy does not accept, a
// statement that breaks the format. The command line reports a refusal with the
// exit code of a rejected run.
package refusal

import (
	"errors"
	"fmt"
)

// ErrRefused is wrapped by every refusal.
var ErrRefused = errors.New("rejected")

// Newf builds a refusal.
func Newf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRefused, fmt.Sprintf(format, args...))
}

// Wrap marks err as a refusal and keeps it in the chain.
func Wrap(err error) error {
	if err == nil || errors.Is(err, ErrRefused) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrRefused, err)
}

// Is reports whether err is, or wraps, a refusal.
func Is(err error) bool {
	return errors.Is(err, ErrRefused)
}
