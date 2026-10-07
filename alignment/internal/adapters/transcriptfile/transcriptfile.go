// Package transcriptfile reads the transcript file a harness leaves for a
// finished subagent, within a size bound.
package transcriptfile

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// MaxBytes bounds a transcript the module reads.
const MaxBytes = 32 << 20

// ErrTooLarge is returned for a transcript beyond MaxBytes.
var ErrTooLarge = errors.New("transcript too large")

// Reader reads transcripts from the file system.
type Reader struct{}

// Read returns the content of the transcript at path.
func (Reader) Read(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open the transcript: %w", err)
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read the transcript: %w", err)
	}
	if len(content) > MaxBytes {
		return nil, ErrTooLarge
	}
	return content, nil
}
