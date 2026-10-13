package promise

import "cmp"

// Category is the kind of call a scan makes to its host.
type Category string

// Categories of the asks of a scan, in the order a scan makes them.
const (
	CategoryExtraction Category = "extraction"
	CategoryWriter     Category = "writer"
	CategoryScope      Category = "scope"
	CategoryGeneration Category = "generation"
	CategoryLabels     Category = "labels"
	// CategoryOther is an emission of the session that no ask of the scan names
	// any more, such as a request whose inputs changed: it spends the budget and
	// is never wanted.
	CategoryOther Category = "other"
)

// categoryOrder ranks the categories by the order a scan makes them.
var categoryOrder = map[Category]int{
	CategoryExtraction: 0,
	CategoryWriter:     1,
	CategoryScope:      2,
	CategoryGeneration: 3,
	CategoryLabels:     4,
	CategoryOther:      5,
}

// Ask is one call a scan makes, or made, to its host. A reissue of a ticket is
// the same ask again: the journal counts it as one more emission.
type Ask struct {
	Category Category
	// Promise is the id of the promise the ask is about; empty for the extraction
	// and the labels.
	Promise string
	// Slot is the index of the writer or of the scope vote within its promise,
	// and the index of a generation ticket within the scan; zero otherwise.
	Slot int
}

// Extraction is the one call that extracts the promises.
func Extraction() Ask { return Ask{Category: CategoryExtraction} }

// Writer is the call of one of the writers of a promise.
func Writer(promise string, slot int) Ask {
	return Ask{Category: CategoryWriter, Promise: promise, Slot: slot}
}

// Scope is the call of one of the scope votes of a promise.
func Scope(promise string, slot int) Ask {
	return Ask{Category: CategoryScope, Promise: promise, Slot: slot}
}

// Generation is the call of one generation ticket, assigned to a promise.
func Generation(promise string, ticket int) Ask {
	return Ask{Category: CategoryGeneration, Promise: promise, Slot: ticket}
}

// LabelTicket is the one call that labels the tests with promises.
func LabelTicket() Ask { return Ask{Category: CategoryLabels} }

// Other is an emission of the session that no ask of the scan names any more.
func Other() Ask { return Ask{Category: CategoryOther} }

// Calls of the budget.
const (
	// ExtractionCalls extract the promises.
	ExtractionCalls = 1
	// WritersPerPromise write the witnesses of one promise.
	WritersPerPromise = 2
	// ScopeVotesPerPromise judge the scope of the witnesses of one promise.
	ScopeVotesPerPromise = 2
	// GenerationTickets generate the model mutants, round-robin over the promises.
	GenerationTickets = 4
	// LabelsCalls label the tests with promises.
	LabelsCalls = 1
	// CallsPerPromise are reserved for a promise before its writers are emitted.
	CallsPerPromise = WritersPerPromise + ScopeVotesPerPromise
	// Budget is how many calls one scan session makes in all, reissues included.
	Budget = ExtractionCalls + MaxPromises*CallsPerPromise + GenerationTickets + LabelsCalls
)

// compareAsks orders asks by category, then promise, then slot.
func compareAsks(a, b Ask) int {
	return cmp.Or(
		cmp.Compare(categoryOrder[a.Category], categoryOrder[b.Category]),
		cmp.Compare(a.Promise, b.Promise),
		cmp.Compare(a.Slot, b.Slot),
	)
}

// compareTickets orders generation tickets by slot, then promise: the order
// they were assigned in.
func compareTickets(a, b Ask) int {
	return cmp.Or(cmp.Compare(a.Slot, b.Slot), cmp.Compare(a.Promise, b.Promise))
}

// AssignGeneration gives each generation ticket to a promise, round-robin over
// the kept promises in their kept order. With no kept promise there is no ticket.
func AssignGeneration(kept []string) []Ask {
	if len(kept) == 0 {
		return nil
	}
	tickets := make([]Ask, GenerationTickets)
	for ticket := range tickets {
		tickets[ticket] = Generation(kept[ticket%len(kept)], ticket)
	}
	return tickets
}

// PromiseAsks lists the writer and scope calls of a promise.
func PromiseAsks(promise string) []Ask {
	asks := make([]Ask, 0, CallsPerPromise)
	for slot := range WritersPerPromise {
		asks = append(asks, Writer(promise, slot))
	}
	for slot := range ScopeVotesPerPromise {
		asks = append(asks, Scope(promise, slot))
	}
	return asks
}
