package task

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// HeldOut is one held-out violation of a task, as the positive control of the
// calibration reads it: its name `<criterion>/<k>`, its criterion, and whether
// the fixture's tests pass on it.
type HeldOut struct {
	Task             string `json:"task"`
	Violation        string `json:"violation"`
	Criterion        string `json:"criterion"`
	FixtureTestsPass bool   `json:"fixtureTestsPass"`
}

// Written is what an artifact was written from: the SHA-256 of the spec and,
// for the citations, of the oracle.
type Written struct {
	Spec   string `json:"spec"`
	Oracle string `json:"oracle,omitempty"`
}

// Provenance is provenance.json: what each oracle and the citations were
// written from.
type Provenance struct {
	Oracle       Written `json:"oracle"`
	SecondOracle Written `json:"second_oracle"`
	Citations    Written `json:"citations"`
}

// ParseProvenance reads provenance.json.
func ParseProvenance(data []byte) (Provenance, error) {
	var provenance Provenance
	if err := json.Unmarshal(data, &provenance); err != nil {
		return Provenance{}, fmt.Errorf("provenance.json: %w", err)
	}
	return provenance, nil
}

// FilesDigest is the SHA-256 of a set of files: each path, length and
// content, in path order.
func FilesDigest(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		fmt.Fprintf(hash, "%s\x00%d\x00", name, len(files[name]))
		hash.Write(files[name])
	}
	return hex.EncodeToString(hash.Sum(nil))
}
