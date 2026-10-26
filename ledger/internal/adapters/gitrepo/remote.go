package gitrepo

import (
	"strings"

	"github.com/RachidChabane/musthold/ledger/internal/domain/ledger"
	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
)

// fetchedLedgerRef holds the remote ledger branch after a fetch. It is a private
// ref: the local ledger branch only ever fast-forwards to it.
const fetchedLedgerRef = "refs/musthold/fetched-ledger"

// Remote synchronises the ledger branch of a repository with a named remote.
type Remote struct {
	repository *Repository
	name       string
}

// NewRemote returns the remote of a repository.
func NewRemote(repository *Repository, name string) *Remote {
	return &Remote{repository: repository, name: name}
}

// Fetch brings the remote ledger branch into the local one. A remote without the
// branch changes nothing. A local branch that is ahead of the remote stays; one
// that has diverged from it is refused, because the ledger never merges.
func (r *Remote) Fetch() error {
	runner := r.repository.runner
	res, err := runner.run(nil, nil, "ls-remote", "--exit-code", r.name, ledgerRef)
	if err != nil {
		if res.exitCode == exitCodeNoMatchingRemoteRef {
			return nil
		}
		return err
	}
	if _, err := runner.output(nil, "fetch", "--quiet", r.name, "+"+ledgerRef+":"+fetchedLedgerRef); err != nil {
		return err
	}
	fetched, err := runner.output(nil, "rev-parse", "--verify", fetchedLedgerRef)
	if err != nil {
		return err
	}
	remoteTip := ledger.ObjectID(strings.TrimSpace(string(fetched)))
	localTip, exists, err := r.repository.Tip()
	if err != nil {
		return err
	}
	if !exists {
		return r.repository.Advance(nil, remoteTip)
	}
	behind, err := runner.answer(exitCodeNegative, "merge-base", "--is-ancestor", string(localTip), string(remoteTip))
	if err != nil {
		return err
	}
	if behind {
		if localTip == remoteTip {
			return nil
		}
		return r.repository.Advance(&localTip, remoteTip)
	}
	ahead, err := runner.answer(exitCodeNegative, "merge-base", "--is-ancestor", string(remoteTip), string(localTip))
	if err != nil {
		return err
	}
	if !ahead {
		return refusal.Newf("the local and the remote ledger have diverged")
	}
	return nil
}

// Push publishes the local ledger branch. The push is never forced, so the remote
// refuses a branch that does not fast-forward.
func (r *Remote) Push() error {
	_, err := r.repository.runner.output(nil, "push", "--quiet", r.name, ledgerRef+":"+ledgerRef)
	return err
}
