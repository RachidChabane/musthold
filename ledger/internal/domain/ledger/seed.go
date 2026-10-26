package ledger

import (
	"encoding/json"
	"strings"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
)

// seedInputSeparator joins the head commit and the run in the seed digest.
const seedInputSeparator = "\n"

// SeedRecord is the seed of a head commit: the digest of the head commit and of
// the first CI run that saw it. It is recorded once, so that re-triggering CI
// cannot shop for another seed.
type SeedRecord struct {
	HeadSHA string `json:"headSha"`
	RunID   string `json:"runId"`
	Seed    string `json:"seed"`
}

// NewSeedRecord derives the seed of a head commit and the first run that saw it.
func NewSeedRecord(headSHA, runID string) (SeedRecord, error) {
	if !headSHAPattern.MatchString(headSHA) {
		return SeedRecord{}, refusal.Newf("%q is not a git commit digest", headSHA)
	}
	if runID == "" || strings.ContainsFunc(runID, func(r rune) bool { return r < ' ' || r == 0x7f }) {
		return SeedRecord{}, refusal.Newf("%q is not a run identifier", runID)
	}
	return SeedRecord{
		HeadSHA: headSHA,
		RunID:   runID,
		Seed:    format.SHA256Hex([]byte(headSHA + seedInputSeparator + runID)),
	}, nil
}

// Bytes returns the canonical bytes of the record, the content of its ledger file.
func (r SeedRecord) Bytes() ([]byte, error) {
	return format.Canonicalize(r)
}

// Addition returns the ledger addition that records the seed. An existing seed
// of the same head commit stays: the first run wins.
func (r SeedRecord) Addition() (Addition, error) {
	content, err := r.Bytes()
	if err != nil {
		return Addition{}, err
	}
	return Addition{Path: SeedPath(r.HeadSHA), Content: content, Policy: PolicyKeepExisting}, nil
}

// ParseSeedRecord reads the content of a seed file and checks that its seed is the
// one its head commit and run give.
func ParseSeedRecord(content []byte) (SeedRecord, error) {
	var record SeedRecord
	if err := json.Unmarshal(content, &record); err != nil {
		return SeedRecord{}, refusal.Newf("seed record: %v", err)
	}
	expected, err := NewSeedRecord(record.HeadSHA, record.RunID)
	if err != nil {
		return SeedRecord{}, err
	}
	if expected != record {
		return SeedRecord{}, refusal.Newf("seed %s is not the digest of its head commit and run", record.Seed)
	}
	canonical, err := record.Bytes()
	if err != nil {
		return SeedRecord{}, err
	}
	if string(canonical) != string(content) {
		return SeedRecord{}, refusal.Newf("seed record is not canonical")
	}
	return record, nil
}
