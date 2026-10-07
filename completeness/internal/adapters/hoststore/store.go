// Package hoststore keeps the host directory of a project on the filesystem: the
// requests, the ticket entries, the accepted responses, the digest-chained
// journal, the secret and the queue. It implements hostproto.HostStore.
//
// Every record other than the journal is written atomically, so a reader never
// sees a partial file. Writers are serialized by an in-process mutex and by an
// advisory lock file shared with the other processes using the same directory.
package hoststore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/RachidChabane/spec-proof/completeness/internal/platform/atomicfile"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Layout of the host directory and the modes of what it holds.
const (
	QueueFile    = "queue.json"
	RequestsDir  = "requests"
	TicketsDir   = "tickets"
	ResponsesDir = "responses"
	JournalFile  = "journal.jsonl"
	SecretFile   = "secret"
	LockFile     = "lock"

	DirMode   os.FileMode = 0o700
	FileMode  os.FileMode = 0o600
	recordExt             = ".json"
	keySep                = "."
)

// ErrInvalidKey is returned when a record is stored under a request id that is
// not a SHA-256 digest.
var ErrInvalidKey = errors.New("invalid record key")

var _ hostproto.HostStore = (*Store)(nil)

// Store is the filesystem implementation of hostproto.HostStore.
type Store struct {
	dir   string
	mutex sync.Mutex
}

// Open creates the host directory and its sub-directories, private to the user,
// and returns the store over them. Opening an existing directory keeps its content.
func Open(dir string) (*Store, error) {
	for _, path := range []string{dir, filepath.Join(dir, RequestsDir), filepath.Join(dir, TicketsDir), filepath.Join(dir, ResponsesDir)} {
		if err := os.MkdirAll(path, DirMode); err != nil {
			return nil, fmt.Errorf("create %s: %w", path, err)
		}
		if err := os.Chmod(path, DirMode); err != nil {
			return nil, fmt.Errorf("restrict %s: %w", path, err)
		}
	}
	return &Store{dir: dir}, nil
}

// Dir returns the host directory.
func (s *Store) Dir() string { return s.dir }

func (s *Store) path(elements ...string) string {
	return filepath.Join(append([]string{s.dir}, elements...)...)
}

// withLock runs fn while holding the in-process mutex and the directory lock.
func withLock[T any](s *Store, fn func() (T, error)) (T, error) {
	var zero T
	s.mutex.Lock()
	defer s.mutex.Unlock()
	release, err := acquireFileLock(s.path(LockFile))
	if err != nil {
		return zero, fmt.Errorf("lock %s: %w", s.dir, err)
	}
	defer release()
	return fn()
}

func inLock(s *Store, fn func() error) error {
	_, err := withLock(s, func() (struct{}, error) { return struct{}{}, fn() })
	return err
}

// readJSON decodes the file at path into target; a missing file is reported as
// notFound.
func readJSON(path string, target any, notFound error) error {
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return notFound
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(content, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	content, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return atomicfile.Write(path, content, FileMode)
}

func createJSON(path string, value any, exists error) error {
	content, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := atomicfile.WriteNew(path, content, FileMode); err != nil {
		if errors.Is(err, atomicfile.ErrExists) {
			return exists
		}
		return err
	}
	return nil
}
