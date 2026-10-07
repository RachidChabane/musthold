package parcel

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Words of the parcel statements that tell a masked criterion token which
// criterion it stands for, and the operations the tests of the project call.
const (
	lineStatementWord     = "unit price"
	discountStatementWord = "discount"
	shippingStatementWord = "shipping fee"
	lineOperation         = "line_total("
	discountOperation     = "discounted("
	shippingOperation     = "shipping_fee("

	testHeader      = "=== Test ==="
	signaturesHead  = "=== Public signatures ==="
	noneChoice      = "NONE"
	coverageID      = "coverage"
	statedPercent   = 95
	statedFraction  = 0.95
	fullCoverage    = "FULL"
	partialCoverage = "PARTIAL"
	weakCoverage    = "WEAK"
)

var tokenLine = regexp.MustCompile(`(?m)^(K-[0-9a-f]+): (.*)$`)

// Reading is how an alignment host reads the masked test of a request: which
// criterion token its assertions check, and how much of the criterion they cover.
type Reading func(unit string, tokens map[string]string) (token, coverage string)

// AlignmentOracle plays an alignment host over the parcel project: it reads the
// criteria and the masked test an executor sees and answers with its reading.
type AlignmentOracle struct {
	Reading Reading
}

// HonestAlignment is the host that reads each test as a careful reader does: a
// test of line totals checks the multiplication criterion fully, a test of a
// discount or of the shipping fee checks a strict subset of its criterion.
func HonestAlignment() AlignmentOracle { return AlignmentOracle{Reading: honestReading} }

// CarelessAlignment is the host that finds the right criterion in every test and
// judges every assertion too weak to catch a violation.
func CarelessAlignment() AlignmentOracle {
	return AlignmentOracle{Reading: func(unit string, tokens map[string]string) (string, string) {
		token, _ := honestReading(unit, tokens)
		if token == noneChoice {
			return noneChoice, noneChoice
		}
		return token, weakCoverage
	}}
}

func honestReading(unit string, tokens map[string]string) (string, string) {
	switch {
	case strings.Contains(unit, lineOperation):
		return tokenOf(tokens, lineStatementWord), fullCoverage
	case strings.Contains(unit, discountOperation):
		return tokenOf(tokens, discountStatementWord), partialCoverage
	case strings.Contains(unit, shippingOperation):
		return tokenOf(tokens, shippingStatementWord), partialCoverage
	}
	return noneChoice, noneChoice
}

func tokenOf(tokens map[string]string, word string) string {
	for token, statement := range tokens {
		if strings.Contains(strings.ToLower(statement), word) {
			return token
		}
	}
	return noneChoice
}

// Answer implements hostprototest.AnswerSource.
func (o AlignmentOracle) Answer(_ string, view hostproto.ExecutorView) (string, error) {
	tokens := map[string]string{}
	for _, match := range tokenLine.FindAllStringSubmatch(view.User, -1) {
		tokens[match[1]] = match[2]
	}
	unit := ""
	if _, after, found := strings.Cut(view.User, testHeader); found {
		unit, _, _ = strings.Cut(after, signaturesHead)
	}
	token, coverage := o.Reading(unit, tokens)
	if len(view.Closed) == 0 {
		return fmt.Sprintf(`{"requirement":%q,"coverage":%q,"confidence":%d}`, token, coverage, statedPercent), nil
	}
	set, err := hostproto.ParseClosedSet(view.Closed)
	if err != nil {
		return "", err
	}
	answers := make([]map[string]any, 0, len(set.Questions))
	for _, question := range set.Questions {
		choice := token
		if question.ID == coverageID {
			choice = coverage
		} else if !offers(question, token) {
			choice = noneChoice
		}
		answers = append(answers, map[string]any{"id": question.ID, "choice": choice, "confidence": statedFraction})
	}
	encoded, err := json.Marshal(map[string]any{"answers": answers})
	return string(encoded), err
}

func offers(question hostproto.ClosedQuestion, token string) bool {
	for _, name := range question.OptionNames() {
		if name == token {
			return true
		}
	}
	return false
}
