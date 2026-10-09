package hostproto

import (
	"errors"
	"fmt"

	"github.com/RachidChabane/spec-proof/format"
)

// ClassifyPlanItem is the plan of one closed request as `systemone classify-plan`
// hands it to the host mod: the ticket and nonce to import the labels with, the
// confidence source the import names, and every call to make, question by
// question, with its model, its text, its labels in the planned order and the
// seed input of that order.
type ClassifyPlanItem struct {
	Ticket          string           `json:"ticket"`
	Nonce           string           `json:"nonce"`
	Source          string           `json:"source"`
	Scheme          string           `json:"scheme"`
	K               int              `json:"k"`
	ClosedSetDigest string           `json:"closedSetDigest"`
	Calls           []ClassifiedCall `json:"calls"`
}

// ErrClassifyVoters is returned when the voters of a classification are not named
// by exactly one of a model and a voter list.
var ErrClassifyVoters = errors.New("name the voters with exactly one of --model and --voters")

// ClassifySourceOf builds the source of a classification of ClassifyCalls calls
// from a single model, which answers every call, or from a voter list
// `<model>*<n>[+<model>*<n>]` whose counts sum to ClassifyCalls.
func ClassifySourceOf(model, voters string) (format.ClassifySource, error) {
	switch {
	case (model == "") == (voters == ""):
		return format.ClassifySource{}, ErrClassifyVoters
	case model != "":
		return format.SingleVoterClassifySource(ClassifyCalls, ClassifyScheme, model)
	}
	listed, err := format.ParseClassifyVoters(voters)
	if err != nil {
		return format.ClassifySource{}, err
	}
	return format.NewClassifySource(ClassifyCalls, ClassifyScheme, listed)
}

// ClassifyPlans plans the classification of the closed requests of a listing that
// no vote answered yet: the first vote of each, as ExportClosed lists them.
func ClassifyPlans(listing Listing, source format.ClassifySource) ([]ClassifyPlanItem, error) {
	items := []ClassifyPlanItem{}
	for _, exported := range ExportClosed(listing) {
		set, err := ParseClosedSet(exported.Closed)
		if err != nil {
			return nil, fmt.Errorf("read the question set of %s: %w", exported.Ticket, err)
		}
		plan, err := set.ClassifyPlan()
		if err != nil {
			return nil, fmt.Errorf("plan %s: %w", exported.Ticket, err)
		}
		calls, err := plan.Calls(source)
		if err != nil {
			return nil, fmt.Errorf("plan %s: %w", exported.Ticket, err)
		}
		items = append(items, ClassifyPlanItem{
			Ticket: exported.Ticket, Nonce: exported.Nonce, Source: source.String(), Scheme: plan.Scheme, K: plan.K,
			ClosedSetDigest: plan.ClosedSetDigest, Calls: calls,
		})
	}
	return items, nil
}
