package ledger_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/ledger"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/refusal"
)

const verifiesSnapshot = " [verifies SP-206~1]"

func timestampOf(version int) []byte {
	return []byte(fmt.Sprintf(`{"signed":{"version":%d}}`, version))
}

func TestSnapshotID(t *testing.T) {
	t.Run("a snapshot is named by the digest of its timestamp file"+verifiesSnapshot, func(t *testing.T) {
		timestamp := timestampOf(1)
		if id := ledger.SnapshotID(timestamp); id != format.SHA256Hex(timestamp) || id == ledger.SnapshotID(timestampOf(2)) {
			t.Fatalf("id %s", id)
		}
	})
}

func TestTimestampVersion(t *testing.T) {
	t.Run("the version of a timestamp file is read"+verifiesSnapshot, func(t *testing.T) {
		if version, err := ledger.TimestampVersion(timestampOf(12)); err != nil || version != 12 {
			t.Fatalf("version %d: %v", version, err)
		}
	})
	for name, document := range map[string]string{
		"not JSON": "{", "no version": `{"signed":{}}`, "version zero": `{"signed":{"version":0}}`, "a version that is not a number": `{"signed":{"version":"2"}}`,
	} {
		t.Run(name+" is refused"+verifiesSnapshot, func(t *testing.T) {
			if _, err := ledger.TimestampVersion([]byte(document)); !errors.Is(err, refusal.ErrRefused) {
				t.Fatal("accepted")
			}
		})
	}
}

func TestNewestSnapshot(t *testing.T) {
	t.Run("the highest timestamp version wins whatever the ids are"+verifiesSnapshot, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			versions := rapid.SliceOfNDistinct(rapid.IntRange(1, 1000), 1, 6, func(version int) int { return version }).Draw(t, "versions")
			timestamps := map[string][]byte{}
			highest, highestID := 0, ""
			for _, version := range versions {
				timestamp := timestampOf(version)
				timestamps[ledger.SnapshotID(timestamp)] = timestamp
				if version > highest {
					highest, highestID = version, ledger.SnapshotID(timestamp)
				}
			}
			id, found, err := ledger.NewestSnapshot(timestamps)
			if err != nil || !found || id != highestID {
				t.Fatalf("newest %s (%v, %v), want %s", id, found, err, highestID)
			}
		})
	})

	t.Run("equal versions are broken by the greater id, so the choice is a function of the contents"+verifiesSnapshot, func(t *testing.T) {
		low, high := strings.Repeat("1", 64), strings.Repeat("9", 64)
		id, found, err := ledger.NewestSnapshot(map[string][]byte{low: timestampOf(5), high: timestampOf(5)})
		if err != nil || !found || id != high {
			t.Fatalf("newest %s (%v, %v)", id, found, err)
		}
	})

	t.Run("no snapshot is not an error"+verifiesSnapshot, func(t *testing.T) {
		if id, found, err := ledger.NewestSnapshot(nil); err != nil || found || id != "" {
			t.Fatalf("newest %q %v %v", id, found, err)
		}
	})

	t.Run("a snapshot with an unreadable timestamp file is refused"+verifiesSnapshot, func(t *testing.T) {
		if _, _, err := ledger.NewestSnapshot(map[string][]byte{strings.Repeat("1", 64): []byte("{")}); err == nil {
			t.Fatal("accepted")
		}
	})
}
