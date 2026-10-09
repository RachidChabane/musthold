package hostproto

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
)

// Parameters of the classification of a closed question by the classifier calls
// of a host.
const (
	// ClassifyCalls is k, the number of calls that answer each question.
	ClassifyCalls = 5
	// ClassifyScheme is the label-order scheme the plan computes.
	ClassifyScheme = format.ClassifySchemeRot1

	classifySeedPrefix    = "host-classify|" + format.ClassifySchemeRot1 + "|"
	classifySeedSeparator = "|"
	// classifySeedBytes is the length of the seed: the first bytes of the SHA-256
	// of the seed input.
	classifySeedBytes = 16
	// classifyWordBytes is the length of each of the two words of the PCG seed.
	classifyWordBytes = classifySeedBytes / 2
	// classifyLowBitMask selects the low bit of the first seed word.
	classifyLowBitMask = 1
)

// ErrClassifyPlan is returned for a question set that cannot be planned.
var ErrClassifyPlan = errors.New("cannot plan the classification")

// Digest is the SHA-256 of the question set: the RFC 8785 canonical form of the
// set as it is marshalled (`state`, then the questions with their `id`, `type`,
// `instructions` and `options`), in lower-case hex.
func (s ClosedSet) Digest() (string, error) {
	canonical, err := format.Canonicalize(s)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrClassifyPlan, err)
	}
	return format.SHA256Hex(canonical), nil
}

// ClassifyCallPlan is one call of a question: the index of the call, the labels in
// the order the call lists them and the text the classifier reads, with the
// options of the question described in the same order.
type ClassifyCallPlan struct {
	Index  int      `json:"i"`
	Labels []string `json:"labels"`
	Text   string   `json:"text"`
}

// ClassifyQuestionPlan is the plan of one question: the seed input and seed that
// order its labels, and its k calls.
type ClassifyQuestionPlan struct {
	ID   string     `json:"question"`
	Type ClosedType `json:"type"`
	// SeedInput is the text the seed is the SHA-256 of.
	SeedInput string `json:"seedInput"`
	// Seed is the first 16 bytes of the SHA-256 of the seed input, in hex.
	Seed  string             `json:"seed"`
	Calls []ClassifyCallPlan `json:"calls"`
}

// ClassifyPlan is the plan of the classifier calls that answer a question set: the
// same order of labels for every model, call by call.
type ClassifyPlan struct {
	Scheme          string                 `json:"scheme"`
	K               int                    `json:"k"`
	ClosedSetDigest string                 `json:"closedSetDigest"`
	Questions       []ClassifyQuestionPlan `json:"questions"`
}

// ClassifyPlan plans the ClassifyCalls calls of every question under the scheme
// `rot1`.
//
// The seed input of a question is "host-classify|rot1|" + the set's Digest + "|" +
// the question id, and its seed the first 16 bytes of the SHA-256 of that text.
// The seed is the two big-endian 64-bit words that seed a math/rand/v2 PCG,
// first word first. A Fisher-Yates shuffle of the canonical order of the
// options, written out as `for i := n-1; i > 0; i--` swapping position i with
// position Uint64N(i+1), gives the permutation pi.
//
// A choice or noul question lists pi rotated by i places for call i: position j
// of call i holds pi[(i+j) mod n]. A score question lists its levels ascending
// or descending: the low bit of the first seed word, 0 or 1, is 1 when call 0
// lists them descending, and the direction alternates with every call.
func (s ClosedSet) ClassifyPlan() (ClassifyPlan, error) {
	if problems := s.Problems(); len(problems) > 0 {
		return ClassifyPlan{}, fmt.Errorf("%w: %w", ErrClassifyPlan, &InvalidError{Problems: problems})
	}
	digest, err := s.Digest()
	if err != nil {
		return ClassifyPlan{}, err
	}
	plan := ClassifyPlan{Scheme: ClassifyScheme, K: ClassifyCalls, ClosedSetDigest: digest}
	for _, question := range s.Questions {
		plan.Questions = append(plan.Questions, planQuestion(s.State, question, digest))
	}
	return plan, nil
}

func planQuestion(state string, question ClosedQuestion, digest string) ClassifyQuestionPlan {
	seedInput := classifySeedPrefix + digest + classifySeedSeparator + question.ID
	sum := sha256.Sum256([]byte(seedInput))
	seed := sum[:classifySeedBytes]
	planned := ClassifyQuestionPlan{ID: question.ID, Type: question.Type, SeedInput: seedInput, Seed: hex.EncodeToString(seed)}
	canonical := question.choiceNames()
	options := optionsByName(question)
	for call := range ClassifyCalls {
		var order []string
		if question.Type == ClosedScore {
			order = scoreOrder(canonical, seed, call)
		} else {
			order = rotatedOrder(permutation(canonical, seed), call)
		}
		planned.Calls = append(planned.Calls, ClassifyCallPlan{Index: call, Labels: order, Text: renderClassifyText(state, question, order, options)})
	}
	return planned
}

func optionsByName(question ClosedQuestion) map[string]string {
	described := make(map[string]string, len(question.Options))
	for _, option := range question.Options {
		described[option.Name] = option.Description
	}
	return described
}

func seedWords(seed []byte) (uint64, uint64) {
	return binary.BigEndian.Uint64(seed[:classifyWordBytes]), binary.BigEndian.Uint64(seed[classifyWordBytes:])
}

// permutation is the Fisher-Yates shuffle of the canonical order the seed gives.
func permutation(canonical []string, seed []byte) []string {
	first, second := seedWords(seed)
	source := rand.New(rand.NewPCG(first, second))
	shuffled := slices.Clone(canonical)
	for position := len(shuffled) - 1; position > 0; position-- {
		other := int(source.Uint64N(uint64(position + 1)))
		shuffled[position], shuffled[other] = shuffled[other], shuffled[position]
	}
	return shuffled
}

func rotatedOrder(shuffled []string, call int) []string {
	rotated := make([]string, len(shuffled))
	for position := range shuffled {
		rotated[position] = shuffled[(call+position)%len(shuffled)]
	}
	return rotated
}

// scoreOrder lists the levels ascending or descending: call 0 is descending when
// the low bit of the first seed word is set, and the direction alternates.
func scoreOrder(ascending []string, seed []byte, call int) []string {
	first, _ := seedWords(seed)
	descending := (first&classifyLowBitMask)^uint64(call&1) == 1
	order := slices.Clone(ascending)
	if descending {
		slices.Reverse(order)
	}
	return order
}

// Words of the text a classifier call reads.
const (
	classifyStateHeading     = "State:"
	classifyQuestionHeading  = "Question:"
	classifyOptionsHeading   = "Options:"
	classifyAscendingHeading = "Levels, lowest first:"
	classifyDescendingHeader = "Levels, highest first:"
)

// renderClassifyText describes the question with its options in the order of the
// call. The options and the labels are permuted together, and no wording of the
// question changes from one call to the next.
func renderClassifyText(state string, question ClosedQuestion, order []string, described map[string]string) string {
	heading := classifyOptionsHeading
	if question.Type == ClosedScore {
		heading = classifyAscendingHeading
		if len(order) > 0 && order[0] != question.Options[0].Name {
			heading = classifyDescendingHeader
		}
	}
	var text strings.Builder
	text.WriteString(classifyStateHeading + "\n" + state + "\n\n" + classifyQuestionHeading + "\n" + question.Instructions + "\n\n" + heading + "\n")
	for _, name := range order {
		text.WriteString("- " + name)
		if description := described[name]; description != "" {
			text.WriteString(": " + description)
		}
		text.WriteString("\n")
	}
	return text.String()
}

// ClassifiedCall is one call of a classification with the model that answers it.
type ClassifiedCall struct {
	Question string   `json:"question"`
	Index    int      `json:"i"`
	Model    string   `json:"model"`
	Text     string   `json:"text"`
	Labels   []string `json:"labels"`
	// SeedInput is the text the order of the labels derives from.
	SeedInput string `json:"seedInput"`
}

// Calls lists every call of the plan, question by question, with the voter that
// answers it.
func (p ClassifyPlan) Calls(source format.ClassifySource) ([]ClassifiedCall, error) {
	if source.K != p.K || source.Scheme != p.Scheme {
		return nil, fmt.Errorf("%w: the source %s does not plan %d calls of %s", ErrClassifyPlan, source, p.K, p.Scheme)
	}
	var calls []ClassifiedCall
	for _, question := range p.Questions {
		for _, call := range question.Calls {
			model, voted := source.ModelOfCall(call.Index)
			if !voted {
				return nil, fmt.Errorf("%w: no voter for call %d", ErrClassifyPlan, call.Index)
			}
			calls = append(calls, ClassifiedCall{
				Question: question.ID, Index: call.Index, Model: model, Text: call.Text,
				Labels: call.Labels, SeedInput: question.SeedInput,
			})
		}
	}
	return calls, nil
}

// Question returns the plan of the question, and whether the plan has it.
func (p ClassifyPlan) Question(id string) (ClassifyQuestionPlan, bool) {
	for _, question := range p.Questions {
		if question.ID == id {
			return question, true
		}
	}
	return ClassifyQuestionPlan{}, false
}
