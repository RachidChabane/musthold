package app

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/modules"
)

// ErrNoTicket is returned when the module's run left no ticket to put an executor on.
var ErrNoTicket = errors.New("the module issued no ticket for an executor")

// Selection is the tickets of a session: the one the executor works on and a
// second one that the no-call cap probe fetches, so that the probe leaves the
// session's own ticket untouched.
type Selection struct {
	Ticket      string
	Role        string
	ProbeTicket string
}

// rolePreference orders the executor roles by how little an executor has to write:
// a closed question needs a one-word answer, a generation needs a document.
var rolePreference = []hostproto.Agent{hostproto.AgentDecider, hostproto.AgentJudge, hostproto.AgentGenerator}

// SelectTickets picks the tickets of a session from the JSON `request list --json`
// printed.
func SelectTickets(listing []byte) (Selection, error) {
	var decoded hostproto.Listing
	if err := json.Unmarshal(listing, &decoded); err != nil {
		return Selection{}, fmt.Errorf("the request listing is not JSON: %w", err)
	}
	plugin := modules.Primary().Plugin
	for _, agent := range rolePreference {
		role := hostproto.RoleFor(plugin, agent)
		var tickets []string
		for _, item := range decoded.Items {
			if item.Role == role {
				tickets = append(tickets, item.View.Ticket)
			}
		}
		if len(tickets) == 0 {
			continue
		}
		selection := Selection{Ticket: tickets[0], Role: role, ProbeTicket: tickets[0]}
		if len(tickets) > 1 {
			selection.ProbeTicket = tickets[len(tickets)-1]
		}
		return selection, nil
	}
	return Selection{}, ErrNoTicket
}
