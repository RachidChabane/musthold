// Package ciseed keeps the seed a CI run drew for a commit, so that a re-run of
// the commit reuses it: re-triggering CI cannot shop for a seed.
package ciseed

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto/hostbundle"
)

const (
	directory             = format.SeedsDirectory
	extension             = ".json"
	dirMode   os.FileMode = 0o755
	fileMode              = 0o644
)

// commitID matches a commit id in hex, SHA-1 or SHA-256.
var commitID = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// Record is what is kept for a commit: the CI run that first saw it and the seed
// drawn from them.
type Record struct {
	HeadSHA    string `json:"headSha"`
	FirstRunID string `json:"firstRunId"`
	Seed       uint64 `json:"seed"`
}

// Resolve returns the seed of a commit: the one already recorded under dir, else
// the one drawn from the commit and this run, which it records.
func Resolve(dir, headSHA, runID string) (Record, error) {
	if !commitID.MatchString(headSHA) {
		return Record{}, fmt.Errorf("%q is not a commit id", headSHA)
	}
	path := filepath.Join(dir, directory, headSHA+extension)
	content, err := os.ReadFile(path)
	switch {
	case err == nil:
		var record Record
		if err := json.Unmarshal(content, &record); err != nil || record.HeadSHA != headSHA {
			return Record{}, fmt.Errorf("the seed record %s is unreadable", path)
		}
		return record, nil
	case !errors.Is(err, fs.ErrNotExist):
		return Record{}, err
	}
	record := Record{HeadSHA: headSHA, FirstRunID: runID, Seed: hostbundle.CISeed(headSHA, runID)}
	encoded, err := json.Marshal(record)
	if err != nil {
		return Record{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return Record{}, err
	}
	return record, os.WriteFile(path, encoded, fileMode)
}
