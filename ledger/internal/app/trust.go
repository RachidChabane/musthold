package app

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
)

// TrustSource fetches a TUF snapshot of the trusted root: every file the TUF client
// read, by path below the mirror.
type TrustSource interface {
	Fetch(ctx context.Context) (map[string][]byte, error)
}

// FetchTrustService fetches a verified TUF snapshot of the trusted root and writes it
// as files, the input of `ledger append --trust`.
type FetchTrustService struct {
	Source TrustSource
	Sink   FileSink
}

// Run fetches the snapshot and writes it. A set without a timestamp file is not a
// snapshot the ledger can name, and is refused.
func (s FetchTrustService) Run(ctx context.Context) (int, error) {
	files, err := s.Source.Fetch(ctx)
	if err != nil {
		return 0, err
	}
	if _, found := files[format.TrustTimestampFile]; !found {
		return 0, refusal.Newf("the mirror served no %s", format.TrustTimestampFile)
	}
	if err := s.Sink.Prepare(); err != nil {
		return 0, err
	}
	for _, path := range slices.Sorted(maps.Keys(files)) {
		if err := s.Sink.WriteFile(path, files[path]); err != nil {
			return 0, fmt.Errorf("write %s: %w", path, err)
		}
	}
	return len(files), nil
}
