package scan

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/promise"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Shape of a session key.
const (
	// bindingKind opens the preimage of every scan session key.
	bindingKind = "scan"
	// sessionKeyLength is how many hex digits of the binding's digest the
	// session key keeps.
	sessionKeyLength = 32
	// differenceJoin joins the differences a refused resume names.
	differenceJoin = "; "
	// absent stands for a value one side of a difference does not have.
	absent = "none"
)

// binding is everything a scan session is bound to: a resume continues a
// session only when every field is equal, since every request of the scan and
// every fact it rests on derive from them.
type binding struct {
	Kind        string          `json:"kind"`
	Commit      string          `json:"commit"`
	Tree        string          `json:"tree"`
	Mode        format.ScanMode `json:"mode"`
	Base        string          `json:"base,omitempty"`
	PullRequest int             `json:"pullRequest,omitempty"`
	Seed        string          `json:"seed"`
	SampleSize  int             `json:"sampleSize"`
	Mutants     int             `json:"mutants"`
	// Budget is the time the mutants may run, and Calls the calls the host may
	// be asked.
	Budget  string `json:"budget"`
	Calls   int    `json:"calls"`
	Version string `json:"moduleVersion"`
	// Sources are the digests of the quotable sources, by name.
	Sources []boundSource `json:"sources"`
}

// boundSource is one quotable source a session is bound to.
type boundSource struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

// newBinding binds a scan to its commit, tree, mode, base, pull request, seed,
// counts, budgets, module version and the digests of its sources.
func newBinding(request Request, commit, tree string, change change, sources []quotableSource) binding {
	bound := binding{
		Kind: bindingKind, Commit: commit, Tree: tree, Mode: format.ScanModeRepository, Base: change.base,
		Seed: strconv.FormatUint(request.Seed, seedBase), SampleSize: request.SampleSize, Mutants: request.Mutants,
		Budget: request.Budget.String(), Calls: promise.Budget, Version: request.Module.Version,
	}
	if change.base != "" {
		bound.Mode = format.ScanModeChange
	}
	if change.pull != nil {
		bound.PullRequest = change.pull.Number
	}
	for _, source := range sources {
		bound.Sources = append(bound.Sources, boundSource{Name: source.name, Digest: format.SHA256Hex(source.content)})
	}
	slices.SortFunc(bound.Sources, func(a, b boundSource) int { return strings.Compare(a.Name, b.Name) })
	return bound
}

// canonical is the binding's canonical encoding: the preimage of its key and
// the detail it is journaled with.
func (b binding) canonical() (string, error) {
	encoded, err := format.Canonicalize(b)
	if err != nil {
		return "", fmt.Errorf("encode the scan's binding: %w", err)
	}
	return string(encoded), nil
}

// sessionKeyOf is the session key of a canonical binding: a prefix of its
// digest.
func sessionKeyOf(canonical string) string {
	return format.SHA256Hex([]byte(canonical))[:sessionKeyLength]
}

// differences names, field by field, what this binding changes from an older
// one, in a fixed order.
func (b binding) differences(older binding) []string {
	var named []string
	compare := func(field, before, after string) {
		if before != after {
			named = append(named, fmt.Sprintf("%s %s → %s", field, orAbsent(before), orAbsent(after)))
		}
	}
	compare("commit", older.Commit, b.Commit)
	compare("tree", older.Tree, b.Tree)
	compare("mode", string(older.Mode), string(b.Mode))
	compare("base", older.Base, b.Base)
	compare("pull request", numberOrEmpty(older.PullRequest), numberOrEmpty(b.PullRequest))
	compare("seed", older.Seed, b.Seed)
	compare("sample", strconv.Itoa(older.SampleSize), strconv.Itoa(b.SampleSize))
	compare("mutants", strconv.Itoa(older.Mutants), strconv.Itoa(b.Mutants))
	compare("budget", older.Budget, b.Budget)
	compare("calls", strconv.Itoa(older.Calls), strconv.Itoa(b.Calls))
	compare("module version", older.Version, b.Version)
	return append(named, sourceDifferences(older.Sources, b.Sources)...)
}

// sourceDifferences names each source added, removed or changed.
func sourceDifferences(older, newer []boundSource) []string {
	digests := func(sources []boundSource) map[string]string {
		byName := map[string]string{}
		for _, source := range sources {
			byName[source.Name] = source.Digest
		}
		return byName
	}
	before, after := digests(older), digests(newer)
	var names []string
	for name := range before {
		names = append(names, name)
	}
	for name := range after {
		if _, known := before[name]; !known {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	var named []string
	for _, name := range names {
		was, had := before[name]
		now, has := after[name]
		switch {
		case !had:
			named = append(named, fmt.Sprintf("source %s added", name))
		case !has:
			named = append(named, fmt.Sprintf("source %s removed", name))
		case was != now:
			named = append(named, fmt.Sprintf("source %s changed", name))
		}
	}
	return named
}

func orAbsent(value string) string {
	if value == "" {
		return absent
	}
	return value
}

func numberOrEmpty(number int) string {
	if number == 0 {
		return ""
	}
	return strconv.Itoa(number)
}

// checkResume accepts a resume when the session is open, and otherwise names
// what differs from the binding of the open scan session journaled last.
func checkResume(store hostproto.HostStore, session hostproto.Session, bound binding) error {
	states, err := store.Sessions()
	if err != nil {
		return fmt.Errorf("read the sessions: %w", err)
	}
	open := map[hostproto.Session]bool{}
	for _, state := range states {
		if state.Session.IsScan() && !state.Ended {
			open[state.Session] = true
		}
	}
	if open[session] {
		return nil
	}
	if len(open) == 0 {
		return ErrNothingToResume
	}
	older, found, err := lastBinding(store, open)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: the open session's binding is unknown", ErrSessionMismatch)
	}
	return fmt.Errorf("%w: %s", ErrSessionMismatch, strings.Join(bound.differences(older), differenceJoin))
}

// lastBinding reads, from the journal, the binding of the open scan session
// bound last.
func lastBinding(store hostproto.HostStore, open map[hostproto.Session]bool) (binding, bool, error) {
	records, err := store.Records()
	if err != nil {
		return binding{}, false, fmt.Errorf("read the journal: %w", err)
	}
	for index := len(records) - 1; index >= 0; index-- {
		event := records[index].Event
		if event.Kind != hostproto.EventSessionBound || !open[event.Session] {
			continue
		}
		var older binding
		if json.Unmarshal([]byte(event.Detail), &older) == nil {
			return older, true, nil
		}
	}
	return binding{}, false, nil
}

// journalBinding journals the canonical binding of the session, once.
func journalBinding(store hostproto.HostStore, session hostproto.Session, canonical string) error {
	records, err := store.Records()
	if err != nil {
		return fmt.Errorf("read the journal: %w", err)
	}
	for _, record := range records {
		if record.Event.Kind == hostproto.EventSessionBound && record.Event.Session == session {
			return nil
		}
	}
	if _, err := store.Append(hostproto.Event{Kind: hostproto.EventSessionBound, Session: session, Detail: canonical}); err != nil {
		return fmt.Errorf("journal the binding of %s: %w", session, err)
	}
	return nil
}
