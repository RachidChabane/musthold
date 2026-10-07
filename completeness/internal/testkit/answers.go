package testkit

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/closedq"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Words of the ledger fixtures that tell an answer what a requirement or a change
// is about.
const (
	statementWord       = "add the amount"
	vagueWord           = "appropriately"
	changeHeader        = "=== Change ==="
	noneChoice          = "NONE"
	missingChoice       = "MISSING"
	coveredChoice       = "COVERED"
	ambiguousChoice     = "AMBIGUOUS"
	testableChoice      = "TESTABLE"
	criterionQuestion   = "criterion"
	tokenPrefix         = "K-"
	surePercent         = 0.95
	missingConfidence   = 0.9
	boundaryConfidence  = 0.3
	vagueConfidence     = 0.8
	noneConfidence      = 0.9
	matchedConfidence   = 0.9
	boundariesDimension = "boundaries"
	errorsDimension     = "error_handling"
)

// Chooser picks the answer to one closed question: the choice and the confidence.
// The state is what the request shows, the requirements and the change; vote is the
// number of the vote the answer is given for.
type Chooser func(question closedq.Question, state string, vote int) (choice string, confidence float64)

// Answerer answers every closed request through a chooser, the way a System One
// model or a decider would.
type Answerer struct {
	Choose Chooser
}

var tokenLine = regexp.MustCompile(`(?m)^(K-[0-9a-f]+): (.*)$`)

// Answer implements hostprototest.AnswerSource.
func (a Answerer) Answer(_ string, view hostproto.ExecutorView) (string, error) {
	var set closedq.Set
	if err := json.Unmarshal(view.Closed, &set); err != nil {
		return "", fmt.Errorf("the request is not a closed request: %w", err)
	}
	ticket, err := hostproto.ParseTicketID(view.Ticket)
	if err != nil {
		return "", err
	}
	answers := make([]string, len(set.Questions))
	for index, question := range set.Questions {
		choice, confidence := a.Choose(question, set.State, ticket.Vote)
		answers[index] = fmt.Sprintf(`{"id":%q,"choice":%q,"confidence":%v}`, question.ID, choice, confidence)
	}
	return `{"answers":[` + strings.Join(answers, ",") + `]}`, nil
}

// statementsOf reads the `token: statement` lines of a state.
func statementsOf(state string) map[string]string {
	statements := map[string]string{}
	for _, match := range tokenLine.FindAllStringSubmatch(state, -1) {
		statements[match[1]] = match[2]
	}
	return statements
}

// Honest is the host that reads the ledger fixtures the way their owner would:
// error handling and boundaries are missing from the spec, the display
// requirement is vague, a discount or a receipt line is behaviour no requirement
// states, and every other change implements the totals requirement.
func Honest() Answerer {
	return Answerer{Choose: func(question closedq.Question, state string, _ int) (string, float64) {
		switch {
		case question.ID == criterionQuestion:
			return honestHunk(question, state)
		case strings.HasPrefix(question.ID, tokenPrefix):
			if strings.Contains(statementsOf(state)[question.ID], vagueWord) {
				return ambiguousChoice, vagueConfidence
			}
			return testableChoice, surePercent
		}
		switch question.ID {
		case errorsDimension:
			return missingChoice, missingConfidence
		case boundariesDimension:
			return missingChoice, boundaryConfidence
		}
		return coveredChoice, surePercent
	}}
}

func honestHunk(question closedq.Question, state string) (string, float64) {
	_, change, _ := strings.Cut(state, changeHeader)
	for _, unrelated := range []string{"apply_discount", "ApplyDiscount", "receipt_line"} {
		if strings.Contains(change, unrelated) {
			return noneChoice, noneConfidence
		}
	}
	statements := statementsOf(state)
	for _, name := range question.OptionNames() {
		if strings.Contains(statements[name], statementWord) {
			return name, matchedConfidence
		}
	}
	return noneChoice, noneConfidence
}
