package ledger

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/refusal"
)

// SnapshotID names a trust snapshot: the SHA-256 of its timestamp file. The
// timestamp file is the one file of a TUF snapshot that is replaced in place when
// the repository refreshes, so each refresh is a new snapshot at paths of its own
// and no path of the ledger is ever rewritten.
func SnapshotID(timestamp []byte) string {
	return format.SHA256Hex(timestamp)
}

// TimestampVersion reads the version of a TUF timestamp file.
func TimestampVersion(timestamp []byte) (int, error) {
	var document struct {
		Signed struct {
			Version int `json:"version"`
		} `json:"signed"`
	}
	if err := json.Unmarshal(timestamp, &document); err != nil {
		return 0, refusal.Newf("timestamp file: %v", err)
	}
	if document.Signed.Version < 1 {
		return 0, refusal.Newf("timestamp file has version %d", document.Signed.Version)
	}
	return document.Signed.Version, nil
}

// NewestSnapshot picks the snapshot an export hands out among the timestamp files
// of the snapshots of the ledger, by snapshot id: the one with the highest
// timestamp version, and the greatest id among equal versions. The choice depends
// on the contents only, so the same ledger always exports the same snapshot.
func NewestSnapshot(timestamps map[string][]byte) (snapshotID string, found bool, err error) {
	type candidate struct {
		id      string
		version int
	}
	candidates := make([]candidate, 0, len(timestamps))
	for id, timestamp := range timestamps {
		version, err := TimestampVersion(timestamp)
		if err != nil {
			return "", false, fmt.Errorf("snapshot %s: %w", id, err)
		}
		candidates = append(candidates, candidate{id: id, version: version})
	}
	if len(candidates) == 0 {
		return "", false, nil
	}
	newest := slices.MaxFunc(candidates, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(a.version, b.version), cmp.Compare(a.id, b.id))
	})
	return newest.id, true, nil
}
