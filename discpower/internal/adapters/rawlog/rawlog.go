// Package rawlog reads the raw API log of the user's host from a directory: the
// files the host writes when OTEL_LOG_RAW_API_BODIES names a directory. It reads
// only inside that directory, following no link out of it, and refuses a file
// larger than the limit it is given.
package rawlog

import (
	"fmt"
	"io"
	"os"
)

// Directory is a raw API log kept in a directory.
type Directory struct{ Root string }

// ReadFile reads one file of the log. A file larger than the limit is refused
// before it is read.
func (d Directory) ReadFile(name string, limit int) ([]byte, error) {
	root, err := os.OpenRoot(d.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > int64(limit) {
		return nil, fmt.Errorf("%s is %d bytes, over %d", name, info.Size(), limit)
	}
	return io.ReadAll(io.LimitReader(file, int64(limit)+1))
}
