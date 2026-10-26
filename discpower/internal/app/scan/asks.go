package scan

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
)

// Tags of the asks of a scan in the queue: `scan-ask/<category>/<promise>/<ticket>`.
// The queue keeps them with the request, so that every emission the journal
// records reads back as the ask it served, whatever the invocation.
const (
	askTagPrefix    = "scan-ask"
	askTagSeparator = "/"
	askTagParts     = 4
)

// tagOf is the queue tag of an ask. The writers and the scope votes of a
// promise are the votes of one request, so their tag leaves the slot out; a
// generation ticket is a request of its own and keeps its ticket.
func tagOf(ask promise.Ask) string {
	ticket := 0
	if ask.Category == promise.CategoryGeneration {
		ticket = ask.Slot
	}
	return strings.Join([]string{askTagPrefix, string(ask.Category), ask.Promise, strconv.Itoa(ticket)}, askTagSeparator)
}

// askOfTag reads the ask a vote of a request with the tag serves.
func askOfTag(tag string, vote int) (promise.Ask, bool) {
	parts := strings.Split(tag, askTagSeparator)
	if len(parts) != askTagParts || parts[0] != askTagPrefix {
		return promise.Ask{}, false
	}
	ticket, err := strconv.Atoi(parts[3])
	if err != nil {
		return promise.Ask{}, false
	}
	category := promise.Category(parts[1])
	switch category {
	case promise.CategoryWriter, promise.CategoryScope:
		return promise.Ask{Category: category, Promise: parts[2], Slot: vote}, true
	case promise.CategoryGeneration:
		return promise.Generation(parts[2], ticket), true
	case promise.CategoryExtraction, promise.CategoryLabels:
		return promise.Ask{Category: category}, true
	}
	return promise.Ask{}, false
}

// hostAsk is one request of a scan with the asks its votes serve: one per vote.
type hostAsk struct {
	asks    []promise.Ask
	request hostbridge.Ask
}

// newHostAsk asks a request of as many votes as the asks it serves, tagged
// with the first.
func newHostAsk(request hostproto.Request, asks ...promise.Ask) hostAsk {
	return hostAsk{asks: asks, request: hostbridge.Ask{
		Request: request, Votes: len(asks), Criteria: []string{tagOf(asks[0])}, Purpose: string(request.Kind),
	}}
}

// asker asks the host what the stages of a scan want, as the budget planner
// allows (promise.Schedule): the planner, never the bridge's emission cap,
// decides what is issued; the cap is a backstop.
type asker struct {
	store  hostproto.HostStore
	bridge *hostbridge.Bridge
	// cuts and unchecked gather what the planner did not issue, over the
	// stages of the invocation.
	cuts      []promise.Cut
	unchecked []promise.Unchecked
	// unasked gathers the asks the emission cap left without a ticket.
	unasked []promise.Ask
}

func newAsker(store hostproto.HostStore, bridge *hostbridge.Bridge) *asker {
	return &asker{store: store, bridge: bridge}
}

// stage plans the asks of one stage against the emissions the session used,
// asks the ones the plan issues and the ones issued before, and gives the
// answer of each, nil for an ask the plan cut. An ask is wanted when one of
// its votes has no ticket yet, whatever ask an earlier request served: a
// request whose inputs changed is asked again.
func (a *asker) stage(asks []hostAsk, kept []string) ([]*hostbridge.Answer, error) {
	used, err := a.used()
	if err != nil {
		return nil, err
	}
	issued := make([]bool, len(asks))
	var wanted []promise.Ask
	for index, ask := range asks {
		missing, err := a.missingVotes(ask)
		if err != nil {
			return nil, err
		}
		issued[index] = len(missing) < len(ask.asks)
		wanted = append(wanted, missing...)
	}
	plan := promise.Schedule(promise.Request{Kept: kept, Used: used, Wanted: wanted})
	a.record(plan)
	answers := make([]*hostbridge.Answer, len(asks))
	for index, ask := range asks {
		if !issued[index] && !slices.ContainsFunc(ask.asks, func(one promise.Ask) bool { return slices.Contains(plan.Issue, one) }) {
			continue
		}
		answer, err := a.want(ask)
		if err != nil {
			return nil, err
		}
		answers[index] = &answer
	}
	return answers, nil
}

// want asks the bridge for a request already planned, as a reissue does.
func (a *asker) want(ask hostAsk) (hostbridge.Answer, error) {
	answer, err := a.bridge.Want(ask.request)
	if err != nil {
		return hostbridge.Answer{}, fmt.Errorf("ask %s: %w", ask.request.Purpose, err)
	}
	for vote, state := range answer.Votes {
		if state.Unasked && vote < len(ask.asks) && !slices.Contains(a.unasked, ask.asks[vote]) {
			a.unasked = append(a.unasked, ask.asks[vote])
		}
	}
	return answer, nil
}

// missingVotes lists the asks of the votes of a request that have no ticket.
func (a *asker) missingVotes(ask hostAsk) ([]promise.Ask, error) {
	id, err := ask.request.Request.ID()
	if err != nil {
		return nil, fmt.Errorf("identify a request: %w", err)
	}
	var missing []promise.Ask
	for vote, one := range ask.asks {
		_, err := a.store.GetEntry(hostproto.Key{RequestID: id, Vote: vote})
		switch {
		case errors.Is(err, hostproto.ErrNotFound):
			missing = append(missing, one)
		case err != nil:
			return nil, fmt.Errorf("read the ticket of %s: %w", id, err)
		}
	}
	return missing, nil
}

// used reads the emissions of the session from the journal, each as the ask
// its request's tag names; an emission no tag names spends the budget all the
// same.
func (a *asker) used() ([]promise.Ask, error) {
	records, err := a.store.Records()
	if err != nil {
		return nil, fmt.Errorf("read the journal: %w", err)
	}
	items, err := a.store.Items()
	if err != nil {
		return nil, fmt.Errorf("read the queue: %w", err)
	}
	tags := map[hostproto.RequestID][]string{}
	for _, item := range items {
		tags[item.RequestID] = item.Criteria
	}
	var used []promise.Ask
	for _, record := range records {
		if record.Event.Kind != hostproto.EventEmitted || record.Event.Session != a.bridge.Session {
			continue
		}
		used = append(used, askOfEmission(record.Event.Ticket, tags))
	}
	return used, nil
}

// askOfEmission reads the ask an emitted ticket served.
func askOfEmission(ticket string, tags map[hostproto.RequestID][]string) promise.Ask {
	id, err := hostproto.ParseTicketID(ticket)
	if err != nil {
		return promise.Other()
	}
	for _, tag := range tags[id.RequestID] {
		if ask, known := askOfTag(tag, id.Vote); known {
			return ask
		}
	}
	return promise.Other()
}

// record keeps what a plan cut and the promises it left unchecked, once each.
func (a *asker) record(plan promise.Plan) {
	for _, cut := range plan.Cut {
		if !slices.Contains(a.cuts, cut) {
			a.cuts = append(a.cuts, cut)
		}
	}
	for _, unchecked := range plan.Unchecked {
		if !slices.Contains(a.unchecked, unchecked) {
			a.unchecked = append(a.unchecked, unchecked)
		}
	}
}

// emissions counts the emissions of the session: the model calls it used.
func (a *asker) emissions() (int, error) {
	records, err := a.store.Records()
	if err != nil {
		return 0, fmt.Errorf("read the journal: %w", err)
	}
	return hostproto.Emissions(records, a.bridge.Session), nil
}

// pending reports whether an answer still waits for a ticket.
func pending(answers ...*hostbridge.Answer) bool {
	return slices.ContainsFunc(answers, func(answer *hostbridge.Answer) bool {
		return answer != nil && answer.Standing == hostbridge.Pending
	})
}

// answered reports whether every vote of an answer holds an answer to read.
func answered(answer *hostbridge.Answer) bool {
	return answer != nil && answer.Standing == hostbridge.Answered
}
