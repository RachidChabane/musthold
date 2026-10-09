package format

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Grammar of the confidence source of a Claude Code classification:
//
//	host-classify(<k>,<scheme>):<model>*<n>[+<model>*<n>]
//
// k is the number of calls per question, scheme names the rule that orders the
// labels of each call, and the voters list the models with the number of calls
// each answered. The counts sum to k and the voters are listed by descending
// count, then by model identifier, so that one source has one spelling.
const (
	// SourceHostClassify is the kind of the confidence of k classifier calls of
	// the host, written `host-classify(<k>,<scheme>):<voters>`.
	SourceHostClassify ConfidenceSource = "host-classify"

	// ClassifySchemeRot1 is the label-order scheme `rot1`: a seeded permutation of
	// the canonical order, rotated by the index of the call.
	ClassifySchemeRot1 = "rot1"

	hostClassifyKind     = string(SourceHostClassify)
	classifyArgSeparator = ","
	classifyVoterJoin    = "+"
	classifyCountMark    = "*"
	classifyVoterColon   = "):"
	// classifyForbiddenInModel are the characters a model identifier must not hold,
	// because the grammar uses them.
	classifyForbiddenInModel = "+*(),: \t\r\n"
)

// ErrClassifySource is returned for a text that is not a host-classify source.
var ErrClassifySource = errors.New("not a host-classify source")

// ClassifySchemes lists the label-order schemes the format defines.
func ClassifySchemes() []string { return []string{ClassifySchemeRot1} }

// ClassifyVoter is one model of a classification and the number of calls it
// answered.
type ClassifyVoter struct {
	Model string
	Calls int
}

// ClassifySource is the parsed form of a host-classify confidence source.
type ClassifySource struct {
	// K is the number of calls per question.
	K int
	// Scheme is the label-order scheme.
	Scheme string
	// Voters are the models, by descending count then identifier.
	Voters []ClassifyVoter
}

// NewClassifySource builds the source of k calls ordered by the scheme that the
// voters answered. The voters are put in their canonical order; their counts
// must sum to k and no model may appear twice.
func NewClassifySource(k int, scheme string, voters []ClassifyVoter) (ClassifySource, error) {
	source := ClassifySource{K: k, Scheme: scheme, Voters: slices.Clone(voters)}
	slices.SortFunc(source.Voters, func(a, b ClassifyVoter) int {
		return cmp.Or(cmp.Compare(b.Calls, a.Calls), strings.Compare(a.Model, b.Model))
	})
	if err := source.check(); err != nil {
		return ClassifySource{}, err
	}
	return source, nil
}

// SingleVoterClassifySource is the source of k calls all answered by one model.
func SingleVoterClassifySource(k int, scheme, model string) (ClassifySource, error) {
	return NewClassifySource(k, scheme, []ClassifyVoter{{Model: model, Calls: k}})
}

func (s ClassifySource) check() error {
	if s.K < 1 {
		return fmt.Errorf("%w: %d calls, want at least 1", ErrClassifySource, s.K)
	}
	if !slices.Contains(ClassifySchemes(), s.Scheme) {
		return fmt.Errorf("%w: %q is not one of the schemes %v", ErrClassifySource, s.Scheme, ClassifySchemes())
	}
	if len(s.Voters) == 0 {
		return fmt.Errorf("%w: no voter", ErrClassifySource)
	}
	total, seen := 0, map[string]bool{}
	for _, voter := range s.Voters {
		switch {
		case voter.Model == "" || strings.ContainsAny(voter.Model, classifyForbiddenInModel):
			return fmt.Errorf("%w: %q is not a model identifier", ErrClassifySource, voter.Model)
		case voter.Calls < 1:
			return fmt.Errorf("%w: %q answered %d calls, want at least 1", ErrClassifySource, voter.Model, voter.Calls)
		case seen[voter.Model]:
			return fmt.Errorf("%w: %q is listed twice", ErrClassifySource, voter.Model)
		}
		seen[voter.Model] = true
		total += voter.Calls
	}
	if total != s.K {
		return fmt.Errorf("%w: the voters answered %d calls, want %d", ErrClassifySource, total, s.K)
	}
	return nil
}

// votersText renders the voters as `<model>*<n>[+<model>*<n>]`.
func (s ClassifySource) votersText() string {
	parts := make([]string, len(s.Voters))
	for index, voter := range s.Voters {
		parts[index] = voter.Model + classifyCountMark + strconv.Itoa(voter.Calls)
	}
	return strings.Join(parts, classifyVoterJoin)
}

// Source renders the confidence source.
func (s ClassifySource) Source() ConfidenceSource {
	return ConfidenceSource(hostClassifyKind + detailOpen + strconv.Itoa(s.K) + classifyArgSeparator + s.Scheme + classifyVoterColon + s.votersText())
}

// String renders the confidence source.
func (s ClassifySource) String() string { return string(s.Source()) }

// ModelOfCall is the model that answers call i: the voters are expanded in their
// listed order, so the first voter answers the first calls.
func (s ClassifySource) ModelOfCall(call int) (string, bool) {
	for _, voter := range s.Voters {
		if call < voter.Calls {
			return voter.Model, true
		}
		call -= voter.Calls
	}
	return "", false
}

// PrimaryModel is the voter with the most calls, the first listed.
func (s ClassifySource) PrimaryModel() string { return s.Voters[0].Model }

// ParseClassifySource reads a host-classify confidence source. It accepts only
// the canonical spelling: voters by descending count, then identifier.
func ParseClassifySource(text string) (ClassifySource, error) {
	rest, found := strings.CutPrefix(text, hostClassifyKind+detailOpen)
	header, voters, split := strings.Cut(rest, classifyVoterColon)
	if !found || !split {
		return ClassifySource{}, fmt.Errorf("%w: %q is not host-classify(<k>,<scheme>):<model>*<n>[+<model>*<n>]", ErrClassifySource, text)
	}
	count, scheme, hasScheme := strings.Cut(header, classifyArgSeparator)
	k, err := strconv.Atoi(count)
	if !hasScheme || err != nil || strconv.Itoa(k) != count {
		return ClassifySource{}, fmt.Errorf("%w: %q has no valid (<k>,<scheme>)", ErrClassifySource, text)
	}
	listed, err := ParseClassifyVoters(voters)
	if err != nil {
		return ClassifySource{}, err
	}
	source, err := NewClassifySource(k, scheme, listed)
	if err != nil {
		return ClassifySource{}, err
	}
	if source.String() != text {
		return ClassifySource{}, fmt.Errorf("%w: %q is not the canonical spelling %q", ErrClassifySource, text, source.String())
	}
	return source, nil
}

// ParseClassifyVoters reads `<model>*<n>[+<model>*<n>]`, in any order.
func ParseClassifyVoters(text string) ([]ClassifyVoter, error) {
	var listed []ClassifyVoter
	for _, part := range strings.Split(text, classifyVoterJoin) {
		model, calls, marked := strings.Cut(part, classifyCountMark)
		number, err := strconv.Atoi(calls)
		if !marked || err != nil || strconv.Itoa(number) != calls {
			return nil, fmt.Errorf("%w: %q is not <model>*<n>", ErrClassifySource, part)
		}
		listed = append(listed, ClassifyVoter{Model: model, Calls: number})
	}
	return listed, nil
}

// HostClassifySource is the confidence source of a classification.
func HostClassifySource(source ClassifySource) ConfidenceSource { return source.Source() }

// MappingHostClassifyPrefix starts the id of the mapping from the modal share of
// a classification to a confidence; the rest is the digest of the table.
const MappingHostClassifyPrefix = "host-classify.share-isotonic" + endpointSeparator

// ErrClassifyMapping is returned for a text that is not a host-classify mapping.
var ErrClassifyMapping = errors.New("not a host-classify mapping")

// HostClassifyMapping is the id of the mapping that reads the confidence from the
// shipped isotonic table whose digest is given.
func HostClassifyMapping(tableDigest string) string { return MappingHostClassifyPrefix + tableDigest }

// ParseHostClassifyMapping reads the table digest out of a mapping id.
func ParseHostClassifyMapping(text string) (string, error) {
	digest, found := strings.CutPrefix(text, MappingHostClassifyPrefix)
	if !found || !IsSHA256Hex(digest) {
		return "", fmt.Errorf("%w: %q is not %s<sha256>", ErrClassifyMapping, text, MappingHostClassifyPrefix)
	}
	return digest, nil
}
