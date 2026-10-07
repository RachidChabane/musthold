package testkit

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Words of the parcel criteria's statements that tell a token which criterion it
// stands for, and what the tests of the fixtures do with them.
const (
	multiplyWord  = "multiply"
	surchargeWord = "remote zone"
	rejectWord    = "reject the line"
	fullCoverage  = "FULL"
	weakCoverage  = "WEAK"
	noneChoice    = "NONE"
	noneCoverage  = "NONE"
	confidencePct = 95
	confidenceFr  = 0.95
	testHeader    = "=== Test ==="
	sigHeader     = "=== Public signatures ==="
)

// Spellings of the same behaviour in the fixtures of every language.
var (
	rejectSpellings   = []string{"raises", "errors.Is", "toThrow", "assertThrows"}
	shippingSpellings = []string{"shipping_cost", "ShippingCost", "shippingCost"}
	weakSpellings     = []string{"is not None", "== 0", "toBeDefined", "assertNotNull"}
)

func mentions(text string, spellings []string) bool {
	for _, spelling := range spellings {
		if strings.Contains(text, spelling) {
			return true
		}
	}
	return false
}

var tokenLine = regexp.MustCompile(`(?m)^(K-[0-9a-f]+): (.*)$`)

// Shown is what an executor sees of a request, read back into its parts.
type Shown struct {
	// Tokens maps each criterion token to the first line of its statement.
	Tokens map[string]string
	// Unit is the masked test.
	Unit string
	// Closed is the question set of a closed request, nil for a judge request.
	Closed *ClosedView
}

// Read parses what an executor sees.
func Read(view hostproto.ExecutorView) Shown {
	shown := Shown{Tokens: map[string]string{}}
	for _, match := range tokenLine.FindAllStringSubmatch(view.User, -1) {
		shown.Tokens[match[1]] = match[2]
	}
	if _, after, found := strings.Cut(view.User, testHeader); found {
		shown.Unit, _, _ = strings.Cut(after, sigHeader)
	}
	if len(view.Closed) > 0 {
		var closed ClosedView
		if err := json.Unmarshal(view.Closed, &closed); err == nil {
			shown.Closed = &closed
		}
	}
	return shown
}

// tokenFor finds the token whose statement holds the word, among the options
// offered when there are any.
func (s Shown) tokenFor(word string) string {
	for token, statement := range s.Tokens {
		if !strings.Contains(statement, word) {
			continue
		}
		if s.Closed == nil || s.Closed.Offers(token) {
			return token
		}
	}
	return ""
}

// Verified is what a careful reader finds the masked test to verify: the token of
// the criterion, or NONE, and how much of it the assertions cover.
func (s Shown) Verified() (token, coverage string) {
	switch {
	case mentions(s.Unit, rejectSpellings):
		token = s.tokenFor(rejectWord)
	case mentions(s.Unit, shippingSpellings):
		token = s.tokenFor(surchargeWord)
	default:
		token = s.tokenFor(multiplyWord)
	}
	if token == "" {
		return noneChoice, noneCoverage
	}
	if mentions(s.Unit, weakSpellings) {
		return token, weakCoverage
	}
	return token, fullCoverage
}

// Oracle is the honest host: it answers every request with what a careful reader
// finds the masked test to verify.
type Oracle struct{}

// Answer implements hostprototest.AnswerSource.
func (Oracle) Answer(_ string, view hostproto.ExecutorView) (string, error) {
	shown := Read(view)
	token, coverage := shown.Verified()
	return answerWith(shown, token, coverage, confidencePct, confidenceFr), nil
}

// Forger is the host that wants every test aligned: it names the first criterion
// of the parcel projects, with full coverage and full confidence, for every test.
type Forger struct{}

// Answer implements hostprototest.AnswerSource.
func (Forger) Answer(_ string, view hostproto.ExecutorView) (string, error) {
	shown := Read(view)
	token := shown.tokenFor(multiplyWord)
	if token == "" {
		return answerWith(shown, noneChoice, noneCoverage, 100, 1), nil
	}
	return answerWith(shown, token, fullCoverage, 100, 1), nil
}

// Fixed answers every judge request with the given token reading and every closed
// request with the criterion that reading names.
type Fixed struct {
	Word     string
	Coverage string
}

// Answer implements hostprototest.AnswerSource.
func (f Fixed) Answer(_ string, view hostproto.ExecutorView) (string, error) {
	shown := Read(view)
	token := shown.tokenFor(f.Word)
	if token == "" {
		return answerWith(shown, noneChoice, noneCoverage, 100, 1), nil
	}
	return answerWith(shown, token, f.Coverage, 100, 1), nil
}

// ClosedView is the question set of a closed request, as a relay or a decider
// reads it.
type ClosedView struct {
	Questions []hostproto.ClosedQuestion `json:"questions"`
}

// Offers reports whether the criterion question offers the token as an option.
func (c ClosedView) Offers(token string) bool {
	for _, question := range c.Questions {
		if question.ID != prompt.QuestionCriterion {
			continue
		}
		for _, option := range question.Options {
			if option.Name == token {
				return true
			}
		}
	}
	return false
}

// answerWith renders the answer to a judge request or to a closed request.
func answerWith(shown Shown, token, coverage string, percent int, fraction float64) string {
	if shown.Closed == nil {
		return fmt.Sprintf(`{"requirement":%q,"coverage":%q,"confidence":%d}`, token, coverage, percent)
	}
	answers := []string{}
	for _, question := range shown.Closed.Questions {
		choice := token
		if question.ID == prompt.QuestionCoverage {
			choice = coverage
		} else if !shown.Closed.Offers(token) {
			choice = noneChoice
		}
		answers = append(answers, fmt.Sprintf(`{"id":%q,"choice":%q,"confidence":%v}`, question.ID, choice, fraction))
	}
	return `{"answers":[` + strings.Join(answers, ",") + `]}`
}
