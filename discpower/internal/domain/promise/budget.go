package promise

import "slices"

// Request is what the planner reads: the promises the scan keeps, in their
// kept order, the emissions the session already used (read from the host
// journal, a reissue being one more emission of the same ask), and the asks
// the current stage wants.
type Request struct {
	Kept   []string
	Used   []Ask
	Wanted []Ask
}

// CutReason says why the planner did not issue an ask.
type CutReason string

// Reasons an ask is cut.
const (
	// CutBudget: the calls left could not cover it at its priority.
	CutBudget CutReason = "budget"
	// CutPromiseNotKept: the ask is about a promise the scan does not keep.
	CutPromiseNotKept CutReason = "promise_not_kept"
)

// Cut is an ask the planner did not issue.
type Cut struct {
	Ask    Ask
	Reason CutReason
}

// Plan is the planner's decision for one stage. Every list is in a fixed
// order, whatever the order of the request.
type Plan struct {
	// Issue are the asks to emit now.
	Issue []Ask
	// Cut are the wanted asks not issued.
	Cut []Cut
	// Unchecked are the promises the budget could not cover.
	Unchecked []Unchecked
}

// Schedule decides which of the wanted asks to issue, so that a scan never
// spends more than Budget calls and the bridge's emission cap is never what
// chooses. The priority, from the last cut to the first:
//   - the extraction;
//   - the writers and the scope votes of each promise, in kept order, whose
//     calls are reserved together before any is emitted; a promise the
//     remaining budget cannot cover is dropped and reported unchecked;
//   - the generation tickets, the last assigned cut first;
//   - the labels ticket, cut first of all.
func Schedule(request Request) Plan {
	scheduler := newScheduler(request)
	scheduler.planExtraction()
	scheduler.planPromises()
	scheduler.planTail()
	scheduler.planStrangers()
	return scheduler.plan
}

type scheduler struct {
	request   Request
	used      map[Ask]bool
	wanted    []Ask
	available int
	plan      Plan
}

func newScheduler(request Request) *scheduler {
	used := make(map[Ask]bool, len(request.Used))
	for _, ask := range request.Used {
		used[ask] = true
	}
	wanted := slices.Clone(request.Wanted)
	slices.SortFunc(wanted, compareAsks)
	return &scheduler{
		request:   request,
		used:      used,
		wanted:    slices.Compact(wanted),
		available: max(0, Budget-len(request.Used)),
	}
}

func (s *scheduler) wantedOf(category Category) []Ask {
	return slices.DeleteFunc(slices.Clone(s.wanted), func(ask Ask) bool { return ask.Category != category })
}

func (s *scheduler) isWanted(ask Ask) bool {
	_, found := slices.BinarySearchFunc(s.wanted, ask, compareAsks)
	return found
}

func (s *scheduler) issue(asks ...Ask) {
	s.plan.Issue = append(s.plan.Issue, asks...)
	s.available -= len(asks)
}

func (s *scheduler) cut(reason CutReason, asks ...Ask) {
	for _, ask := range asks {
		s.plan.Cut = append(s.plan.Cut, Cut{Ask: ask, Reason: reason})
	}
}

func (s *scheduler) planExtraction() {
	asks := s.wantedOf(CategoryExtraction)
	if len(asks) == 0 {
		return
	}
	if len(asks) > s.available {
		s.cut(CutBudget, asks...)
		return
	}
	s.issue(asks...)
}

func (s *scheduler) planPromises() {
	for _, id := range s.request.Kept {
		group := s.promiseWanted(id)
		reserved := s.unissuedCanonical(id)
		if len(group) == 0 && !s.started(id) {
			continue
		}
		if len(group)+reserved > s.available {
			s.cut(CutBudget, group...)
			s.plan.Unchecked = append(s.plan.Unchecked, Unchecked{Promise: id, Reason: UncheckedBudget})
			continue
		}
		s.issue(group...)
		s.available -= reserved
	}
}

func (s *scheduler) promiseWanted(id string) []Ask {
	return slices.DeleteFunc(slices.Clone(s.wanted), func(ask Ask) bool {
		return ask.Promise != id || (ask.Category != CategoryWriter && ask.Category != CategoryScope)
	})
}

// unissuedCanonical counts the calls of a promise neither used nor wanted now:
// the calls the promise still needs in a later stage, reserved from the start.
func (s *scheduler) unissuedCanonical(id string) int {
	count := 0
	for _, ask := range PromiseAsks(id) {
		if !s.used[ask] && !s.isWanted(ask) {
			count++
		}
	}
	return count
}

func (s *scheduler) started(id string) bool {
	return slices.ContainsFunc(PromiseAsks(id), func(ask Ask) bool { return s.used[ask] })
}

// planTail issues the generation tickets in assignment order and then the
// labels ticket, stopping at the first one the budget cannot cover: what is cut
// is always the tail of that order.
func (s *scheduler) planTail() {
	tickets := s.wantedOf(CategoryGeneration)
	slices.SortFunc(tickets, compareTickets)
	tail := slices.Concat(s.keptOnly(tickets), s.wantedOf(CategoryLabels))
	covered := true
	for _, ask := range tail {
		if covered && s.available >= 1 {
			s.issue(ask)
			continue
		}
		covered = false
		s.cut(CutBudget, ask)
	}
}

// keptOnly keeps the asks about a promise the scan keeps.
func (s *scheduler) keptOnly(asks []Ask) []Ask {
	return slices.DeleteFunc(slices.Clone(asks), func(ask Ask) bool { return !slices.Contains(s.request.Kept, ask.Promise) })
}

// planStrangers cuts the wanted asks about a promise the scan does not keep.
func (s *scheduler) planStrangers() {
	for _, ask := range s.wanted {
		if ask.Promise != "" && !slices.Contains(s.request.Kept, ask.Promise) {
			s.cut(CutPromiseNotKept, ask)
		}
	}
}
