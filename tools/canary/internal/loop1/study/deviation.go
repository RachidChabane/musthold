package study

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// DeviationsFile is the deviation log of the work directory, and DeviationsTitle
// its first line (PREREG §12).
const (
	DeviationsFile  = "DEVIATIONS.md"
	DeviationsTitle = "# Deviations"
	// DeviationTimestampsDir holds one timestamped file per protocol or
	// token-exposed entry.
	DeviationTimestampsDir = "deviation-timestamps"
)

// Kind is the closed set of deviation kinds.
type Kind string

// The kinds of deviation.
const (
	KindProvenance     Kind = "provenance"
	KindQuotaStop      Kind = "quota-stop"
	KindHarnessError   Kind = "harness-error"
	KindQuotaBlocked   Kind = "quota-blocked"
	KindHarnessBlocked Kind = "harness-blocked"
	KindTokenExposed   Kind = "token-exposed"
	KindProtocol       Kind = "protocol"
)

// Kinds lists every kind, in the order of the protocol's table.
func Kinds() []Kind {
	return []Kind{KindProvenance, KindQuotaStop, KindHarnessError, KindQuotaBlocked, KindHarnessBlocked, KindTokenExposed, KindProtocol}
}

// Timestamped reports whether an entry of this kind is stored with an RFC 3161
// token.
func (k Kind) Timestamped() bool { return k == KindProtocol || k == KindTokenExposed }

// The scopes that name no run or task.
const (
	ScopeNone = "none"
	ScopeAll  = "all"
)

// Deviation is one entry of the deviation log.
type Deviation struct {
	Time time.Time
	Kind Kind
	// Scope is ScopeNone, ScopeAll, or run or task names.
	Scope []string
	// Section is the section of the protocol, without its sign, such as "6.4".
	Section string
	// Says is what the protocol says; Happened is what happened.
	Says     string
	Happened string
	// Raw is set when the line did not have the entry's shape: it is then a
	// protocol entry with scope all, and Raw is the line.
	Raw string
}

const (
	fieldSeparator = " | "
	entryPrefix    = "- "
	sectionSign    = "§"
	scopeSeparator = ","
	entryFields    = 6
)

// ErrDeviation is returned for an entry that cannot be written.
var ErrDeviation = errors.New("the deviation cannot be written")

// Line formats an entry as one line of the log.
func (d Deviation) Line() (string, error) {
	if !slices.Contains(Kinds(), d.Kind) {
		return "", fmt.Errorf("%w: kind %q is not one of %v", ErrDeviation, d.Kind, Kinds())
	}
	if len(d.Scope) == 0 || d.Section == "" || d.Says == "" || d.Happened == "" {
		return "", fmt.Errorf("%w: scope, section, what the protocol says and what happened are required", ErrDeviation)
	}
	for _, field := range append([]string{d.Section, d.Says}, d.Scope...) {
		if strings.Contains(field, fieldSeparator) || strings.ContainsAny(field, "\n\r") {
			return "", fmt.Errorf("%w: %q holds a field separator or a line break", ErrDeviation, field)
		}
	}
	if strings.ContainsAny(d.Happened, "\n\r") {
		return "", fmt.Errorf("%w: what happened holds a line break", ErrDeviation)
	}
	return entryPrefix + strings.Join([]string{
		d.Time.UTC().Format(time.RFC3339),
		string(d.Kind),
		strings.Join(d.Scope, scopeSeparator),
		sectionSign + d.Section,
		d.Says,
		d.Happened,
	}, fieldSeparator), nil
}

// ParseDeviations reads the log. Its title and blank lines are skipped; any other
// line that does not have the entry's shape is a protocol entry with scope all.
func ParseDeviations(text string) []Deviation {
	var entries []Deviation
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == DeviationsTitle {
			continue
		}
		entries = append(entries, parseLine(trimmed))
	}
	return entries
}

func parseLine(line string) Deviation {
	raw := Deviation{Kind: KindProtocol, Scope: []string{ScopeAll}, Raw: line}
	if !strings.HasPrefix(line, entryPrefix) {
		return raw
	}
	fields := strings.SplitN(strings.TrimPrefix(line, entryPrefix), fieldSeparator, entryFields)
	if len(fields) != entryFields {
		return raw
	}
	at, err := time.Parse(time.RFC3339, fields[0])
	kind := Kind(fields[1])
	if err != nil || !slices.Contains(Kinds(), kind) || !strings.HasPrefix(fields[3], sectionSign) || fields[2] == "" {
		return raw
	}
	return Deviation{
		Time:     at.UTC(),
		Kind:     kind,
		Scope:    strings.Split(fields[2], scopeSeparator),
		Section:  strings.TrimPrefix(fields[3], sectionSign),
		Says:     fields[4],
		Happened: fields[5],
	}
}
