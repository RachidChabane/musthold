// Package recordings keeps the answers of a run in a directory, one file per
// vote of a request: `<name>.json` holds the output, in the form a command-line
// fake host reads for the first vote, and `<name>.executor.json` the profile of
// the executor that gave it. The name of the first vote is the request id, the
// name of another vote the request id and the vote number.
package recordings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
	"github.com/RachidChabane/spec-proof/completeness/internal/platform/atomicfile"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	outputSuffix   = ".json"
	executorSuffix = ".executor.json"
	directoryMode  = 0o755
	fileMode       = 0o644
	voteSeparator  = "."
	voteBase       = 10
	voteBits       = 31
)

// ErrInvalidRequestID is returned for a name that is not a request id, nor a
// request id followed by a vote number.
var ErrInvalidRequestID = errors.New("not a recording name")

// Directory is a directory of recordings.
type Directory struct {
	Dir string
}

var _ ports.Recordings = Directory{}

func (d Directory) path(name, suffix string) (string, error) {
	if !validName(name) {
		return "", fmt.Errorf("%w: %q", ErrInvalidRequestID, name)
	}
	return filepath.Join(d.Dir, name+suffix), nil
}

// validName accepts a request id, optionally followed by a dot and a vote number.
func validName(name string) bool {
	id, vote, hasVote := strings.Cut(name, voteSeparator)
	if !format.IsSHA256Hex(id) {
		return false
	}
	if !hasVote {
		return true
	}
	_, err := strconv.ParseUint(vote, voteBase, voteBits)
	return err == nil
}

// Load reads the recording of a request. The executor is nil when none was
// recorded.
func (d Directory) Load(requestID string) (ports.Recording, error) {
	outputPath, err := d.path(requestID, outputSuffix)
	if err != nil {
		return ports.Recording{}, err
	}
	output, err := os.ReadFile(outputPath)
	if err != nil {
		return ports.Recording{}, fmt.Errorf("read the recording: %w", err)
	}
	recording := ports.Recording{Output: output}
	executorPath, err := d.path(requestID, executorSuffix)
	if err != nil {
		return ports.Recording{}, err
	}
	raw, err := os.ReadFile(executorPath)
	if errors.Is(err, os.ErrNotExist) {
		return recording, nil
	}
	if err != nil {
		return ports.Recording{}, fmt.Errorf("read the recorded executor: %w", err)
	}
	var executor format.ExecutorProfile
	if err := json.Unmarshal(raw, &executor); err != nil {
		return ports.Recording{}, fmt.Errorf("decode the recorded executor: %w", err)
	}
	recording.Executor = &executor
	return recording, nil
}

// Record keeps the output and the executor of a request, replacing an earlier
// recording of it.
func (d Directory) Record(requestID string, output []byte, executor format.ExecutorProfile) error {
	outputPath, err := d.path(requestID, outputSuffix)
	if err != nil {
		return err
	}
	executorPath, err := d.path(requestID, executorSuffix)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d.Dir, directoryMode); err != nil {
		return fmt.Errorf("create the recordings directory: %w", err)
	}
	encoded, err := json.Marshal(executor)
	if err != nil {
		return err
	}
	if err := atomicfile.Write(outputPath, output, fileMode); err != nil {
		return fmt.Errorf("write the recording: %w", err)
	}
	return atomicfile.Write(executorPath, encoded, fileMode)
}
