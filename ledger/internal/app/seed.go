package app

import (
	"fmt"

	"github.com/RachidChabane/musthold/ledger/internal/domain/ledger"
)

// SeedService finds the seed of a head commit: the one the ledger already holds,
// or the one the first run of the head commit gives.
type SeedService struct {
	Ref   LedgerRef
	Files LedgerFiles
}

// Resolve returns the recorded seed of headSHA when the ledger has one, and
// otherwise the seed runID would record. Recorded tells which of the two it is.
func (s SeedService) Resolve(headSHA, runID string) (seed ledger.SeedRecord, recorded bool, err error) {
	tip, exists, err := s.Ref.Tip()
	if err != nil {
		return ledger.SeedRecord{}, false, fmt.Errorf("read the ledger tip: %w", err)
	}
	if exists {
		seed, recorded, err = s.recorded(tip, headSHA)
		if err != nil || recorded {
			return seed, recorded, err
		}
	}
	seed, err = ledger.NewSeedRecord(headSHA, runID)
	return seed, false, err
}

func (s SeedService) recorded(tip ledger.ObjectID, headSHA string) (ledger.SeedRecord, bool, error) {
	files, err := s.Files.Files(tip)
	if err != nil {
		return ledger.SeedRecord{}, false, fmt.Errorf("list the ledger: %w", err)
	}
	wanted := ledger.SeedPath(headSHA)
	for _, file := range files {
		if file.Path != wanted {
			continue
		}
		content, err := s.Files.ReadBlob(file.ID)
		if err != nil {
			return ledger.SeedRecord{}, false, fmt.Errorf("read %s: %w", file.Path, err)
		}
		record, err := ledger.ParseSeedRecord(content)
		return record, err == nil, err
	}
	return ledger.SeedRecord{}, false, nil
}
