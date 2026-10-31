package prove

import (
	"fmt"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

// writePending tells Shallnot what the module still waits for: the criteria
// whose tickets wait for the host, and those whose outcome rests on a ticket the
// host abandoned; and how many host answers the module accepted.
func (u UseCase) writePending(result Result) error {
	if u.Deps.Pending == nil {
		return nil
	}
	records, err := u.Deps.Store.Records()
	if err != nil {
		return fmt.Errorf("read the journal: %w", err)
	}
	pending := format.Pending{Module: u.Settings.PendingModule, Pending: []format.PendingEntry{}, Answered: hostproto.AcceptedAnswers(records)}
	for _, criterion := range result.Criteria {
		if reason, waits := pendingReason(criterion); waits {
			pending.Pending = append(pending.Pending, format.PendingEntry{Criterion: criterion.Ref.String(), Reason: reason})
		}
	}
	if err := u.Deps.Pending.Write(pending); err != nil {
		return fmt.Errorf("write the pending file: %w", err)
	}
	return nil
}

// pendingReason is why the criterion has no proof to keep yet, if it has none.
func pendingReason(criterion CriterionResult) (format.PendingReason, bool) {
	switch {
	case criterion.Status == StatusAwaitingHost:
		return format.PendingAwaitingHost, true
	case criterion.Outcome.Abandoned():
		return format.PendingHostUnresponsive, true
	}
	return "", false
}
