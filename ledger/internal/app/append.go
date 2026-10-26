package app

import (
	"errors"
	"fmt"

	"github.com/RachidChabane/musthold/ledger/internal/domain/ledger"
)

// AppendService adds files to the ledger branch as one fast-forward commit.
type AppendService struct {
	Ref       LedgerRef
	Committer Committer
	Remote    LedgerRemote
	Payloads  ledger.PayloadReader
	Clock     Clock
}

// AppendReport says what a run did to the ledger.
type AppendReport struct {
	// Commit is the new tip, or the unchanged one when nothing was added; it is
	// empty when the ledger has no commit at all.
	Commit    ledger.ObjectID
	Added     []string
	Unchanged []string
}

// Run checks every addition, then writes one commit on top of the ledger tip. A path
// that exists with other bytes refuses the whole run before anything is written to
// the branch. With nothing new to add no commit is made; the remote is still
// brought in sync, so a run that failed to push is completed by running it again.
func (s AppendService) Run(additions []ledger.Addition) (AppendReport, error) {
	if err := checkAdditions(additions, s.Payloads); err != nil {
		return AppendReport{}, err
	}
	if err := s.Remote.Fetch(); err != nil {
		return AppendReport{}, fmt.Errorf("fetch the ledger: %w", err)
	}
	tip, exists, err := s.Ref.Tip()
	if err != nil {
		return AppendReport{}, fmt.Errorf("read the ledger tip: %w", err)
	}
	var previous *ledger.ObjectID
	var root *ledger.ObjectID
	if exists {
		previous = &tip
		tree, err := s.Ref.TreeOf(tip)
		if err != nil {
			return AppendReport{}, fmt.Errorf("read the ledger tree: %w", err)
		}
		root = &tree
	}
	tree, outcome, err := ledger.ApplyAdditions(s.Committer, root, additions)
	if err != nil {
		return AppendReport{}, err
	}
	report := AppendReport{Commit: tip, Added: outcome.Added, Unchanged: outcome.Unchanged}
	if len(outcome.Added) > 0 {
		commit, err := s.Committer.Commit(tree, previous, commitMessage(outcome.Added), s.Clock.Now())
		if err != nil {
			return AppendReport{}, fmt.Errorf("commit: %w", err)
		}
		if err := s.Ref.Advance(previous, commit); err != nil {
			return AppendReport{}, fmt.Errorf("advance the ledger: %w", err)
		}
		report.Commit = commit
	}
	if report.Commit != "" {
		if err := s.Remote.Push(); err != nil {
			return report, fmt.Errorf("push the ledger: %w", err)
		}
	}
	return report, nil
}

func checkAdditions(additions []ledger.Addition, payloads ledger.PayloadReader) error {
	var problems []error
	for _, addition := range additions {
		if err := ledger.CheckFile(addition.Path, addition.Content, payloads); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

func commitMessage(added []string) string {
	counts := map[ledger.Category]int{}
	for _, path := range added {
		counts[ledger.Classify(path)]++
	}
	return fmt.Sprintf("ledger: add %d proofs, %d mutant sets, %d seeds, %d trust files",
		counts[ledger.CategoryProof], counts[ledger.CategoryMutant], counts[ledger.CategorySeed], counts[ledger.CategoryTrust])
}
