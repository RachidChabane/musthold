package filelock

import (
	"errors"
	"path/filepath"
	"testing"
)

const lockMode = 0o600

func TestTry(t *testing.T) {
	t.Run("a held lock is refused without waiting and taken again once released", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "run.lock")
		release, err := Try(path, lockMode)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Try(path, lockMode); !errors.Is(err, ErrHeld) {
			t.Fatalf("a second holder: %v", err)
		}
		release()
		again, err := Try(path, lockMode)
		if err != nil {
			t.Fatalf("after the release: %v", err)
		}
		again()
	})
}
