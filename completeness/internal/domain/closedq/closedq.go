// Package closedq is the one place that knows the shape of a closed request, the
// `decide.closed` kind of the host protocol: the question set a System One
// endpoint or a host decider is shown, the schema its answer must satisfy, how
// the answer is parsed and how the votes of a request are read into one answer
// per question with the source of its confidence.
//
// The shape mirrors the closed-question protocol of `/v1/systemone`. It lives in
// this package alone, so that moving it to the shared format library touches no
// other file.
package closedq

import (
	"cmp"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// The question set, its schema and its answers are the host protocol's: one
// definition for every module.
type (
	// Option is one answer of a closed question with its criteria text.
	Option = hostproto.ClosedOption
	// Question is one closed question.
	Question = hostproto.ClosedQuestion
	// Set is the question set of a closed request.
	Set = hostproto.ClosedSet
	// Answer is the answer to one closed question.
	Answer = hostproto.ClosedAnswer
)

// TypeChoice is the question type of a choice among named options.
const TypeChoice = hostproto.ClosedChoice

// MaxQuestions is the largest number of questions one request holds.
const MaxQuestions = hostproto.MaxClosedQuestions

// Choice builds a choice question.
func Choice(id, instructions string, options []Option) Question {
	return hostproto.ChoiceQuestion(id, instructions, options)
}

// Parse reads an output that satisfies the schema into the answers by question id.
func Parse(output []byte) (map[string]Answer, error) { return hostproto.ParseClosedAnswers(output) }

// Vote is one answer to a closed request as the module reads it.
type Vote struct {
	Output []byte
	// Relayed is true for the answer of a System One endpoint; it stands for the
	// votes of the host.
	Relayed bool
	// Source names the model and endpoint of a relayed answer.
	Source string
}

// Decided is what the votes of a request settled for one question.
type Decided struct {
	Choice string
	// Confidence is the confidence the endpoint stated, none counting as zero, or
	// the share of the votes that gave the choice.
	Confidence float64
}

// Reading is what the votes of one request say.
type Reading struct {
	// Questions maps a question id to the decided answer; a question nobody
	// answered is absent.
	Questions map[string]Decided
	// Source says where the confidence came from: the model of the relay, or
	// `host-votes(k)` for k host votes; Mapping is the id of the mapping that
	// derived it.
	Source  format.ConfidenceSource
	Mapping string
}

// HostVotesSource names the confidence source of k host votes.
func HostVotesSource(votes int) format.ConfidenceSource { return format.HostVotesSource(votes) }

// Read turns the votes of a closed request into one answer per question. A
// relayed first vote replaces the host votes and carries the confidence the
// endpoint gave. Host votes give the modal choice of each question, ties to the
// smaller choice, with its share of all votes as confidence; a vote that does not
// parse counts for no choice.
func Read(votes []Vote) Reading {
	if len(votes) == 0 {
		return Reading{Questions: map[string]Decided{}}
	}
	if votes[0].Relayed {
		return readRelayed(votes[0])
	}
	counts := map[string]map[string]int{}
	for _, vote := range votes {
		parsed, err := Parse(vote.Output)
		if err != nil {
			continue
		}
		for id, answer := range parsed {
			if counts[id] == nil {
				counts[id] = map[string]int{}
			}
			counts[id][answer.Choice]++
		}
	}
	reading := Reading{Questions: map[string]Decided{}, Source: HostVotesSource(len(votes)), Mapping: format.MappingModalShare}
	for id, choices := range counts {
		choice, count := modalOf(choices)
		reading.Questions[id] = Decided{Choice: choice, Confidence: float64(count) / float64(len(votes))}
	}
	return reading
}

func readRelayed(vote Vote) Reading {
	reading := Reading{Questions: map[string]Decided{}, Source: format.SystemOneSource(vote.Source), Mapping: hostproto.ClosedChoice.ConfidenceMapping()}
	parsed, err := Parse(vote.Output)
	if err != nil {
		return reading
	}
	for id, answer := range parsed {
		reading.Questions[id] = Decided{Choice: answer.Choice, Confidence: statedConfidence(answer)}
	}
	return reading
}

func statedConfidence(answer Answer) float64 {
	if answer.HasConfidence {
		return answer.Confidence
	}
	return 0
}

// modalOf returns the commonest choice and its count, ties going to the
// lexicographically smaller choice.
func modalOf(counts map[string]int) (string, int) {
	choices := make([]string, 0, len(counts))
	for choice := range counts {
		choices = append(choices, choice)
	}
	slices.SortFunc(choices, func(a, b string) int { return cmp.Or(cmp.Compare(counts[b], counts[a]), strings.Compare(a, b)) })
	return choices[0], counts[choices[0]]
}
