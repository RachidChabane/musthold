// Package refusal marks the errors that are a decision of the ledger, as opposed
// to a failure of the tool: a statement that breaks the format, a path that would
// be rewritten, a history that is not append-only. The command line reports a
// refusal with the exit code of a blocked run.
package refusal

import (
	"errors"
	"fmt"
)

// ErrRefused is wrapped by every refusal.
var ErrRefused = errors.New("refused")

// Newf builds a refusal.
func Newf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRefused, fmt.Sprintf(format, args...))
}

// Is reports whether err is, or wraps, a refusal.
func Is(err error) bool {
	return errors.Is(err, ErrRefused)
}
