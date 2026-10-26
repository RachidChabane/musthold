package hostproto

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/RachidChabane/musthold/format"
)

// ErrClassifyTable is returned for a share table or an admission table that
// breaks its rules.
var ErrClassifyTable = errors.New("invalid classification table")

// SharePoint is one step of an isotonic table: the confidence of a modal answer
// that gathered Count of the k calls.
type SharePoint struct {
	Count      int     `json:"count"`
	Confidence float64 `json:"confidence"`
}

// ShareTable maps the modal count of a classification to a confidence: a
// non-decreasing step function fitted on measured correctness, the value at a
// count being that of the greatest point at or below it. A count below every
// point has no confidence.
type ShareTable struct {
	K      int          `json:"k"`
	Scheme string       `json:"scheme"`
	Points []SharePoint `json:"points"`
}

// Validate checks that the table steps up through counts of 1 to k and that its
// confidences lie in [0,1] and never decrease.
func (t ShareTable) Validate() error {
	if t.K < 1 {
		return fmt.Errorf("%w: %d calls", ErrClassifyTable, t.K)
	}
	if !slices.Contains(format.ClassifySchemes(), t.Scheme) {
		return fmt.Errorf("%w: %q is not a scheme", ErrClassifyTable, t.Scheme)
	}
	if len(t.Points) == 0 {
		return fmt.Errorf("%w: no point", ErrClassifyTable)
	}
	previous := SharePoint{}
	for index, point := range t.Points {
		switch {
		case point.Count < 1 || point.Count > t.K:
			return fmt.Errorf("%w: the count %d of point %d is outside 1 to %d", ErrClassifyTable, point.Count, index, t.K)
		case index > 0 && point.Count <= previous.Count:
			return fmt.Errorf("%w: the counts must increase, %d follows %d", ErrClassifyTable, point.Count, previous.Count)
		case point.Confidence < format.MinConfidence || point.Confidence > format.MaxConfidence:
			return fmt.Errorf("%w: the confidence %v of point %d is outside [%v,%v]", ErrClassifyTable, point.Confidence, index, format.MinConfidence, format.MaxConfidence)
		case index > 0 && point.Confidence < previous.Confidence:
			return fmt.Errorf("%w: the confidence falls from %v to %v at the count %d", ErrClassifyTable, previous.Confidence, point.Confidence, point.Count)
		}
		previous = point
	}
	return nil
}

// Digest is the SHA-256 of the canonical form of the table.
func (t ShareTable) Digest() (string, error) {
	canonical, err := format.Canonicalize(t)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrClassifyTable, err)
	}
	return format.SHA256Hex(canonical), nil
}

// ConfidenceAt is the confidence of a modal answer of the given count, and
// whether the table has one.
func (t ShareTable) ConfidenceAt(count int) (float64, bool) {
	confidence, found := 0.0, false
	for _, point := range t.Points {
		if point.Count > count {
			break
		}
		confidence, found = point.Confidence, true
	}
	return confidence, found
}

// ClassifyOutcomeMet is the outcome an admission entry records: the rule that
// admits the classification held on the held-out half.
const ClassifyOutcomeMet = "MET"

// ClassifyAdmissionEntry admits one classification: the decision kind, the exact
// source, the model of the session that made the calls, the table that was
// tested with its digest, and the version of the host it was measured on.
type ClassifyAdmissionEntry struct {
	Kind        format.Kind             `json:"kind"`
	Source      format.ConfidenceSource `json:"source"`
	Model       string                  `json:"model"`
	Outcome     string                  `json:"outcome"`
	CLIVersion  string                  `json:"cliVersion"`
	Table       ShareTable              `json:"table"`
	TableDigest string                  `json:"tableDigest"`
}

// ClassifyAdmissionKey is what an entry admits.
type ClassifyAdmissionKey struct {
	Kind   format.Kind
	Source format.ConfidenceSource
	Model  string
}

// Key is the key of the entry.
func (e ClassifyAdmissionEntry) Key() ClassifyAdmissionKey {
	return ClassifyAdmissionKey{Kind: e.Kind, Source: e.Source, Model: e.Model}
}

func (e ClassifyAdmissionEntry) validate() error {
	spec, err := format.ParseClassifySource(string(e.Source))
	switch {
	case !slices.Contains(format.Kinds(), e.Kind):
		return fmt.Errorf("%w: %q is not a proof kind", ErrClassifyTable, e.Kind)
	case err != nil:
		return fmt.Errorf("%w: %v", ErrClassifyTable, err)
	case e.Model == "":
		return fmt.Errorf("%w: the entry for %s names no session model", ErrClassifyTable, e.Source)
	case e.Outcome != ClassifyOutcomeMet:
		return fmt.Errorf("%w: the outcome %q admits nothing", ErrClassifyTable, e.Outcome)
	case e.CLIVersion == "":
		return fmt.Errorf("%w: the entry for %s names no CLI version", ErrClassifyTable, e.Source)
	case e.Table.K != spec.K || e.Table.Scheme != spec.Scheme:
		return fmt.Errorf("%w: the table of %s is for %d calls of %s", ErrClassifyTable, e.Source, e.Table.K, e.Table.Scheme)
	}
	if err := e.Table.Validate(); err != nil {
		return err
	}
	digest, err := e.Table.Digest()
	if err != nil {
		return err
	}
	if digest != e.TableDigest {
		return fmt.Errorf("%w: the table of %s has the digest %s, the entry says %s", ErrClassifyTable, e.Source, digest, e.TableDigest)
	}
	return nil
}

// ClassifyAdmission is the table of classifications the product answers with.
type ClassifyAdmission struct {
	Entries []ClassifyAdmissionEntry `json:"entries"`
}

// ParseClassifyAdmission reads an admission table and checks every entry.
func ParseClassifyAdmission(data []byte) (ClassifyAdmission, error) {
	var table ClassifyAdmission
	if err := json.Unmarshal(data, &table); err != nil {
		return ClassifyAdmission{}, fmt.Errorf("%w: %v", ErrClassifyTable, err)
	}
	seen := map[ClassifyAdmissionKey]bool{}
	for _, entry := range table.Entries {
		if err := entry.validate(); err != nil {
			return ClassifyAdmission{}, err
		}
		if seen[entry.Key()] {
			return ClassifyAdmission{}, fmt.Errorf("%w: %s is admitted twice for %s on %s", ErrClassifyTable, entry.Source, entry.Kind, entry.Model)
		}
		seen[entry.Key()] = true
	}
	return table, nil
}

// Lookup returns the entry that admits the classification, and whether there is one.
func (a ClassifyAdmission) Lookup(key ClassifyAdmissionKey) (ClassifyAdmissionEntry, bool) {
	for _, entry := range a.Entries {
		if entry.Key() == key {
			return entry, true
		}
	}
	return ClassifyAdmissionEntry{}, false
}

//go:embed classify_admission.json
var shippedClassifyAdmission []byte

// ShippedClassifyAdmission is the admission table the product ships.
func ShippedClassifyAdmission() (ClassifyAdmission, error) {
	return ParseClassifyAdmission(shippedClassifyAdmission)
}
