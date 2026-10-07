package format

import (
	"strconv"
	"strings"
)

// ConfidenceSource tells where the confidence of a decision came from, so that
// thresholds and calibration are keyed per source (ADR 0005, "System One chain").
// It is a `kind` or a `kind:detail` string; build it with the constructors.
type ConfidenceSource string

// Kinds of confidence source.
const (
	// SourceExecution is a decision that rests on execution or construction: the
	// confidence is the maximum, or the minimum when nothing could decide.
	SourceExecution ConfidenceSource = "execution"

	hostVotesKind = "host-votes"
	systemOneKind = "system-one"
	detailOpen    = "("
	detailClose   = ")"
	detailColon   = ":"
)

// HostVotesSource is the confidence of k blind votes of the host: the modal share
// or the weakest stated confidence of the votes.
func HostVotesSource(k int) ConfidenceSource {
	return ConfidenceSource(hostVotesKind + detailOpen + strconv.Itoa(k) + detailClose)
}

// SystemOneSource is the confidence a System One endpoint reported; origin names
// the model and the endpoint, as `<model>@<endpointDigest>`.
func SystemOneSource(origin string) ConfidenceSource {
	return ConfidenceSource(systemOneKind + detailColon + origin)
}

// IsKnown reports whether the source has one of the forms above.
func (s ConfidenceSource) IsKnown() bool {
	text := string(s)
	switch {
	case s == SourceExecution:
		return true
	case strings.HasPrefix(text, hostVotesKind+detailOpen) && strings.HasSuffix(text, detailClose):
		votes, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(text, hostVotesKind+detailOpen), detailClose))
		return err == nil && votes >= 1
	case strings.HasPrefix(text, systemOneKind+detailColon):
		return len(text) > len(systemOneKind+detailColon)
	}
	return false
}

// Ids of the mappings that turn what an answerer gave into a recorded confidence.
const (
	// MappingSystemOneChoice is the endpoint's own `confidence` of a choice answer.
	MappingSystemOneChoice = "system-one.choice.confidence"
	// MappingSystemOneNoul is `max(p, 1-p)` of the probability of a noul answer.
	MappingSystemOneNoul = "system-one.noul.max-p"
	// MappingSystemOneScore is the endpoint's own `confidence` of a score answer.
	MappingSystemOneScore = "system-one.score.confidence"
	// MappingModalShare is the share of the host votes that agree on the modal answer.
	MappingModalShare = "host-votes.modal-share"
	// MappingWeakestStated is the lowest confidence the host's judges stated.
	MappingWeakestStated = "host-votes.weakest-stated"
)
