package operator

import (
	"fmt"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
)

// Site is where a mutation applies: the unit it edits and the byte span of the
// mutated node inside the original file.
type Site struct {
	File     string
	Qualname string
	Target   mutation.TargetKind
	Span     ecosystem.Span
}

// Key identifies the site.
func (s Site) Key() string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%010d\x00%010d", s.File, s.Target, s.Qualname, s.Span.Start, s.Span.End)
}

// Origin tells an operator mutant from a sentinel.
type Origin string

// Mutant origins.
const (
	OriginOperator Origin = "operator"
	OriginSentinel Origin = "sentinel"
)

// Candidate is a mutation an engine proposes, before identification.
type Candidate struct {
	Site   Site
	Family Family
	// Replaces is the span of the original unit that Edit.NewSource replaces; for
	// a module addition it is the empty span at the end of the file.
	Replaces ecosystem.Span
	// Original is the source of the unit before the change; empty for a
	// module addition.
	Original string
	Edit     mutation.Edit
}

// Mutant is an identified candidate. Sentinels carry Origin OriginSentinel and
// a SentinelKind instead of a Family.
type Mutant struct {
	ID       string
	EditSet  mutation.EditSet
	Family   Family
	Site     Site
	Replaces ecosystem.Span
	Original string
	Origin   Origin
	Sentinel SentinelKind
}

// IsSentinel reports whether the mutant is a sentinel.
func (m Mutant) IsSentinel() bool { return m.Origin == OriginSentinel }

// NewMutant identifies an operator candidate by the digest of its edit set.
func NewMutant(c Candidate) (Mutant, error) {
	set := mutation.EditSet{Edits: []mutation.Edit{c.Edit}}
	id, err := set.ID()
	if err != nil {
		return Mutant{}, err
	}
	return Mutant{ID: id, EditSet: set, Family: c.Family, Site: c.Site, Replaces: c.Replaces, Original: c.Original, Origin: OriginOperator}, nil
}
