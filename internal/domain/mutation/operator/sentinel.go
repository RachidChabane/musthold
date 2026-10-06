package operator

import (
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/internal/domain/mutation"
)

// SentinelKind is an entry of the catalogue of AST-level semantic no-ops.
type SentinelKind string

// The catalogue.
const (
	// SentinelDeadAssignment assigns a fresh local that nothing reads.
	SentinelDeadAssignment SentinelKind = "dead_assignment"
	// SentinelAlphaRename renames a local variable consistently.
	SentinelAlphaRename SentinelKind = "alpha_rename"
	// SentinelUnusedConstant adds a private module constant nothing references.
	SentinelUnusedConstant SentinelKind = "unused_constant"
)

var sentinelKinds = []SentinelKind{SentinelDeadAssignment, SentinelAlphaRename, SentinelUnusedConstant}

// SentinelKinds lists the catalogue in a stable order.
func SentinelKinds() []SentinelKind { return slices.Clone(sentinelKinds) }

// SentinelCandidate is a sentinel an engine proposes for a file.
type SentinelCandidate struct {
	Kind     SentinelKind
	Site     Site
	Replaces ecosystem.Span
	Original string
	Edit     mutation.Edit
}

// DrawSentinels draws SentinelsPerFile sentinels for one file by seed. Kinds are
// visited in a seeded order and the candidate of each kind is drawn by seed, so a
// file gets two different kinds whenever two are available. The draw is a pure
// function of the candidates as a set, the file and the seed.
func DrawSentinels(file string, candidates []SentinelCandidate, seed Seed) ([]Mutant, error) {
	byKind := map[SentinelKind][]SentinelCandidate{}
	for _, c := range candidates {
		byKind[c.Kind] = append(byKind[c.Kind], c)
	}
	var kinds []SentinelKind
	for _, kind := range sentinelKinds {
		if len(byKind[kind]) > 0 {
			kinds = append(kinds, kind)
			slices.SortFunc(byKind[kind], compareSentinels)
		}
	}
	rng := streamFor(seed, "sentinel\x00"+file)
	shuffle(rng, kinds)
	for _, kind := range kinds {
		shuffle(rng, byKind[kind])
	}

	var drawn []Mutant
	for round := 0; len(drawn) < SentinelsPerFile; round++ {
		progressed := false
		for _, kind := range kinds {
			if round >= len(byKind[kind]) || len(drawn) >= SentinelsPerFile {
				continue
			}
			progressed = true
			mutant, err := newSentinel(byKind[kind][round])
			if err != nil {
				return nil, err
			}
			drawn = append(drawn, mutant)
		}
		if !progressed {
			break
		}
	}
	return drawn, nil
}

func compareSentinels(a, b SentinelCandidate) int {
	return strings.Compare(a.Site.Key()+a.Edit.NewSource, b.Site.Key()+b.Edit.NewSource)
}

func newSentinel(c SentinelCandidate) (Mutant, error) {
	set := mutation.EditSet{Edits: []mutation.Edit{c.Edit}}
	id, err := set.ID()
	if err != nil {
		return Mutant{}, err
	}
	return Mutant{ID: id, EditSet: set, Site: c.Site, Replaces: c.Replaces, Original: c.Original, Origin: OriginSentinel, Sentinel: c.Kind}, nil
}
