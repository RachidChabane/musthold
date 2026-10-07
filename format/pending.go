package format

import (
	"cmp"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// PendingReason says why a criterion has no proof yet.
type PendingReason string

// Pending reasons, the vocabulary Shallnot's gate reads from the manifest.
const (
	PendingAwaitingHost         PendingReason = "awaiting_host"
	PendingAwaitingProof        PendingReason = "awaiting_proof"
	PendingAwaitingHostJudgment PendingReason = "awaiting_host_judgment"
	PendingHostUnresponsive     PendingReason = "host_unresponsive"
)

var pendingReasons = []PendingReason{
	PendingAwaitingHost, PendingAwaitingProof, PendingAwaitingHostJudgment, PendingHostUnresponsive,
}

// PendingReasons returns the closed set of pending reasons.
func PendingReasons() []PendingReason { return append([]PendingReason(nil), pendingReasons...) }

// PendingDirectory and PendingFileExtension locate the pending file of a module:
// `.spec-proof/pending/<module>.json`, next to the cache and the verified set.
const (
	PendingDirectory     = "pending"
	PendingFileExtension = ".json"
)

// PendingEntry is one criterion a module could not prove yet.
type PendingEntry struct {
	// Criterion is `ID~REV`.
	Criterion string        `json:"criterion"`
	Reason    PendingReason `json:"reason"`
}

// Pending is the content of a module's pending file: the criteria the module
// still waits on, strictly ascending by criterion, then reason. The verifier
// folds the files of every module into the manifest it writes.
type Pending struct {
	Module  string         `json:"module"`
	Pending []PendingEntry `json:"pending"`
}

// IsCriterion reports whether name is a criterion `ID~REV`: a requirement ID
// without separator and a revision from MinimumRevision on.
func IsCriterion(name string) bool {
	id, revision, found := strings.Cut(name, RevisionSeparator)
	if !found || id == "" || strings.Contains(revision, RevisionSeparator) {
		return false
	}
	number, err := strconv.Atoi(revision)
	return err == nil && number >= MinimumRevision && strconv.Itoa(number) == revision
}

// DecodePending decodes a pending file and validates it. Unknown members are
// ignored. A violation is reported as a *ValidationError listing every one.
func DecodePending(data []byte) (Pending, error) {
	var pending Pending
	if err := json.Unmarshal(data, &pending); err != nil {
		return Pending{}, fmt.Errorf("decode pending file: %w", err)
	}
	if err := pending.Validate(); err != nil {
		return Pending{}, err
	}
	return pending, nil
}

// Validate checks the module name, every entry and the entry order.
func (p Pending) Validate() error {
	var sink collector
	if !IsPathSegment(p.Module) {
		sink.add(CodeMissingField, "module", "%q is not a module name", p.Module)
	}
	for index, entry := range p.Pending {
		path := fmt.Sprintf("pending[%d]", index)
		if !IsCriterion(entry.Criterion) {
			sink.add(CodeRequirement, path+".criterion", "%q is not a criterion ID~REV", entry.Criterion)
		}
		checkMember(&sink, path+".reason", entry.Reason, pendingReasons)
	}
	checkAscending(&sink, "pending", p.Pending, comparePendingEntries)
	return sink.err()
}

func comparePendingEntries(a, b PendingEntry) int {
	return cmp.Or(cmp.Compare(a.Criterion, b.Criterion), cmp.Compare(a.Reason, b.Reason))
}
