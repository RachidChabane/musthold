package hoststore

import (
	"errors"
	"fmt"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/platform/filelock"
	"github.com/RachidChabane/musthold/format/hostproto"
)

// Run locks of the sessions: one file per session next to the store's own lock.
const (
	runLockPrefix    = "run-"
	runLockSuffix    = ".lock"
	sessionSeparator = ":"
	fileSeparator    = "-"
)

// LockRun implements hostproto.RunLock with an advisory lock on the session's
// run file, taken without waiting, so that a second run of the session, in this
// process or another, is refused while the first holds it.
func (s *Store) LockRun(session hostproto.Session) (func(), error) {
	if err := session.Validate(); err != nil {
		return nil, err
	}
	name := runLockPrefix + strings.ReplaceAll(string(session), sessionSeparator, fileSeparator) + runLockSuffix
	release, err := filelock.Try(s.path(name), FileMode)
	switch {
	case errors.Is(err, filelock.ErrHeld):
		return nil, hostproto.ErrSessionBusy
	case err != nil:
		return nil, fmt.Errorf("lock the run of %s: %w", session, err)
	}
	return release, nil
}
