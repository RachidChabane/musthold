package record

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Files reads records directories.
type Files struct{}

// Load reads the records of a counted attempt; see Load.
func (Files) Load(name, records, project string) (Run, map[string]string, error) {
	return Load(name, records, project)
}

// Load reads the records of a counted attempt named name: every file loop-1's
// facts read, first-stop-oracle.json when the session made an end-of-turn call
// and adjudication.json for an escape. It returns the record and the SHA-256 of
// every file it read, by its path relative to the records directory.
func Load(name, records, project string) (Run, map[string]string, error) {
	in, digests, err := facts.Load(records, project)
	if err != nil {
		return Run{}, nil, err
	}
	var firstStop *study.Oracle
	if err := decodeOptional(records, FirstStopOracleFile, digests, &firstStop); err != nil {
		return Run{}, nil, err
	}
	var adjudication *Adjudication
	if err := decodeOptional(records, AdjudicationFile, digests, &adjudication); err != nil {
		return Run{}, nil, err
	}
	run, err := Compose(name, in, firstStop, adjudication)
	if err != nil {
		return Run{}, nil, err
	}
	return run, digests, nil
}

// decodeOptional decodes a file of the records into target, a pointer that
// stays nil when the file is absent, and records the file's SHA-256.
func decodeOptional[T any](records, file string, digests map[string]string, target **T) error {
	path := filepath.Join(records, file)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	digest := sha256.Sum256(data)
	digests[file] = hex.EncodeToString(digest[:])
	*target = &value
	return nil
}
