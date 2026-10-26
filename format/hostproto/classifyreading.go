package hostproto

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/format"
)

// TauClassify is the confidence every question of a request needs for a
// classification to answer it. A request with one question below it goes to the
// host votes.
const TauClassify = 0.95

// Most attempts a call makes: a call that came back without a label, or was
// rejected, is made once more with the same labels in the same order.
const MaxClassifyAttempts = 2

// MaxClassifyReportBytes is the largest report an import reads.
const MaxClassifyReportBytes = MaxOutputBytes

// ErrClassifyReport is returned for a report that does not match the plan it
// reports on.
var ErrClassifyReport = errors.New("invalid classification report")

// ReportedCall is the result of one call as the host mod reports it: the label
// the model returned, nil when the call produced none, and how many attempts
// were made.
type ReportedCall struct {
	Question string  `json:"question"`
	Index    int     `json:"i"`
	Model    string  `json:"model"`
	Label    *string `json:"label"`
	Attempts int     `json:"attempts"`
	// Rejected is set when the host rejected the last attempt of the call, which
	// is a missing call and not a missing vote.
	Rejected bool `json:"rejected,omitempty"`
}

// ClassifyReport is what the host mod hands `systemone import`: the calls it
// made, the version of the host and the model of its session.
type ClassifyReport struct {
	CLIVersion   string         `json:"cliVersion"`
	SessionModel string         `json:"sessionModel"`
	Calls        []ReportedCall `json:"calls"`
}

// ParseClassifyReport reads a report, refusing unknown members.
func ParseClassifyReport(data []byte) (ClassifyReport, error) {
	var report ClassifyReport
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return ClassifyReport{}, fmt.Errorf("%w: %v", ErrClassifyReport, err)
	}
	if report.CLIVersion == "" || report.SessionModel == "" {
		return ClassifyReport{}, fmt.Errorf("%w: cliVersion and sessionModel are required", ErrClassifyReport)
	}
	return report, nil
}

// QuestionReading is what the calls of one question settled.
type QuestionReading struct {
	Question string
	// Modal is the label most calls returned, empty when there is none or when two
	// labels tied for it.
	Modal string
	// Votes is the number of calls that returned the modal label.
	Votes int
	// Share is Votes over all calls of the question, missing ones included.
	Share float64
	// Tied is set when two labels share the top count: there is no prediction.
	Tied bool
	// Rejected counts the calls the host rejected: the question is missing a call
	// and has no prediction.
	Rejected int
	// Calibrated is set when the table has a confidence for the count.
	Calibrated bool
	// Confidence is the table's value at the count, zero without a prediction.
	Confidence float64
}

// Predicts reports whether the question has a modal label with a confidence.
func (q QuestionReading) Predicts() bool { return q.Modal != "" && !q.Tied && q.Calibrated }

// ClassifyGate is the reason a classification does not answer its request.
type ClassifyGate string

// Reasons a classification does not answer.
const (
	// GateOpen: every question has a confidence of at least TauClassify.
	GateOpen ClassifyGate = ""
	// GateTie: a question has no prediction because two labels tied.
	GateTie ClassifyGate = "tie"
	// GateRejected: the host rejected a call of a question, which is then missing.
	GateRejected ClassifyGate = "rejected_call"
	// GateNoVotes: a question got no label from any call.
	GateNoVotes ClassifyGate = "no_votes"
	// GateUncalibrated: the table has no confidence for the count of a question.
	GateUncalibrated ClassifyGate = "uncalibrated"
	// GateBelowTau: a question has a confidence below TauClassify.
	GateBelowTau ClassifyGate = "below_tau"
)

// ClassifyReading is the reading of a whole classification.
type ClassifyReading struct {
	Questions []QuestionReading
	Gate      ClassifyGate
	Record    format.ClassifyRecord
}

// Answers reports whether the classification answers its request.
func (r ClassifyReading) Answers() bool { return r.Gate == GateOpen }

// ReadClassification reads the report against the plan of the set under the
// source: it checks that the report holds every planned call once with the
// planned model and a planned label, counts the labels of each question over all
// calls, leaves a tie without a prediction, takes the confidence of the modal
// count from the table, and opens the gate only when every question has a
// confidence of at least TauClassify. The record holds every call, the seed
// input of every question and the version of the host.
func ReadClassification(requestID RequestID, set ClosedSet, source format.ClassifySource, report ClassifyReport, table ShareTable) (ClassifyReading, error) {
	plan, err := set.ClassifyPlan()
	if err != nil {
		return ClassifyReading{}, err
	}
	planned, err := plan.Calls(source)
	if err != nil {
		return ClassifyReading{}, err
	}
	calls, err := matchReportedCalls(planned, report.Calls)
	if err != nil {
		return ClassifyReading{}, err
	}
	reading := ClassifyReading{Gate: GateOpen}
	reading.Record = format.ClassifyRecord{
		RequestID: string(requestID), Source: source.Source(), Mapping: "", CLIVersion: report.CLIVersion,
		SessionModel: report.SessionModel, ClosedSetDigest: plan.ClosedSetDigest, Calls: calls,
	}
	digest, err := table.Digest()
	if err != nil {
		return ClassifyReading{}, err
	}
	reading.Record.Mapping = format.HostClassifyMapping(digest)
	for _, question := range plan.Questions {
		read := readQuestion(question, calls, source.K, table)
		reading.Questions = append(reading.Questions, read)
		reading.Record.Questions = append(reading.Record.Questions, format.ClassifyQuestionRecord{
			Question: question.ID, SeedInput: question.SeedInput, Seed: question.Seed, Modal: read.Modal,
			Votes: read.Votes, Share: read.Share, Tied: read.Tied, Confidence: read.Confidence,
		})
		reading.Gate = firstGate(reading.Gate, gateOf(read))
	}
	return reading, nil
}

func firstGate(current, next ClassifyGate) ClassifyGate {
	if current != GateOpen {
		return current
	}
	return next
}

func gateOf(read QuestionReading) ClassifyGate {
	switch {
	case read.Rejected > 0:
		return GateRejected
	case read.Tied:
		return GateTie
	case read.Modal == "":
		return GateNoVotes
	case !read.Calibrated:
		return GateUncalibrated
	case read.Confidence < TauClassify:
		return GateBelowTau
	}
	return GateOpen
}

// matchReportedCalls pairs each planned call with its reported one and builds the
// records, in the order of the plan.
func matchReportedCalls(planned []ClassifiedCall, reported []ReportedCall) ([]format.ClassifyCallRecord, error) {
	if len(reported) != len(planned) {
		return nil, fmt.Errorf("%w: %d calls reported, %d planned", ErrClassifyReport, len(reported), len(planned))
	}
	type callKey struct {
		question string
		index    int
	}
	byKey := make(map[callKey]ReportedCall, len(reported))
	for _, call := range reported {
		key := callKey{call.Question, call.Index}
		if _, twice := byKey[key]; twice {
			return nil, fmt.Errorf("%w: call %d of %q is reported twice", ErrClassifyReport, call.Index, call.Question)
		}
		byKey[key] = call
	}
	records := make([]format.ClassifyCallRecord, len(planned))
	for position, call := range planned {
		got, reportedCall := byKey[callKey{call.Question, call.Index}]
		switch {
		case !reportedCall:
			return nil, fmt.Errorf("%w: call %d of %q is not reported", ErrClassifyReport, call.Index, call.Question)
		case got.Model != call.Model:
			return nil, fmt.Errorf("%w: call %d of %q was made by %q, planned for %q", ErrClassifyReport, call.Index, call.Question, got.Model, call.Model)
		case got.Attempts < 1 || got.Attempts > MaxClassifyAttempts:
			return nil, fmt.Errorf("%w: call %d of %q made %d attempts, want 1 to %d", ErrClassifyReport, call.Index, call.Question, got.Attempts, MaxClassifyAttempts)
		case got.Rejected && got.Label != nil:
			return nil, fmt.Errorf("%w: call %d of %q is rejected and returns the label %q", ErrClassifyReport, call.Index, call.Question, *got.Label)
		case got.Label != nil && !slices.Contains(call.Labels, *got.Label):
			return nil, fmt.Errorf("%w: call %d of %q returned %q, which is none of %v", ErrClassifyReport, call.Index, call.Question, *got.Label, call.Labels)
		}
		records[position] = format.ClassifyCallRecord{
			Question: call.Question, Index: call.Index, Model: call.Model, Order: slices.Clone(call.Labels),
			Label: got.Label, Attempts: got.Attempts, Rejected: got.Rejected,
		}
	}
	return records, nil
}

// readQuestion counts the labels the calls of a question returned.
func readQuestion(question ClassifyQuestionPlan, calls []format.ClassifyCallRecord, k int, table ShareTable) QuestionReading {
	counts := map[string]int{}
	read := QuestionReading{Question: question.ID}
	for _, call := range calls {
		if call.Question != question.ID {
			continue
		}
		if call.Rejected {
			read.Rejected++
		}
		if call.Label != nil {
			counts[*call.Label]++
		}
	}
	if len(counts) == 0 || read.Rejected > 0 {
		return withVotesOnly(read, counts, k)
	}
	labels := make([]string, 0, len(counts))
	for label := range counts {
		labels = append(labels, label)
	}
	slices.SortFunc(labels, func(a, b string) int { return cmp.Or(cmp.Compare(counts[b], counts[a]), strings.Compare(a, b)) })
	read.Modal, read.Votes = labels[0], counts[labels[0]]
	read.Share = float64(read.Votes) / float64(k)
	read.Tied = len(labels) > 1 && counts[labels[1]] == read.Votes
	if read.Tied {
		read.Modal = ""
		return read
	}
	read.Confidence, read.Calibrated = table.ConfidenceAt(read.Votes)
	return read
}

// Output renders the answers of a classification that opened the gate as the
// output document of a response to the set: each question's modal label with the
// table's confidence, and, for a choice or noul question, the probability the
// confidence stands for on its modal label with the rest shared evenly.
func (r ClassifyReading) Output(set ClosedSet) ([]byte, error) {
	if !r.Answers() {
		return nil, fmt.Errorf("%w: the classification does not answer its request (%s)", ErrClassifyReport, r.Gate)
	}
	answers := make(map[string]ClosedAnswer, len(set.Questions))
	byID := make(map[string]QuestionReading, len(r.Questions))
	for _, read := range r.Questions {
		byID[read.Question] = read
	}
	for _, question := range set.Questions {
		read := byID[question.ID]
		answer := ClosedAnswer{Choice: read.Modal, Confidence: read.Confidence, HasConfidence: true}
		if question.Type != ClosedScore {
			answer.Probabilities = modalProbabilities(question, read)
		}
		answers[question.ID] = answer
	}
	return set.AnswerDocument(answers)
}

func modalProbabilities(question ClosedQuestion, read QuestionReading) map[string]float64 {
	names := question.choiceNames()
	probabilities := make(map[string]float64, len(names))
	rest := (1 - read.Confidence) / float64(len(names)-1)
	for _, name := range names {
		probabilities[name] = rest
	}
	probabilities[read.Modal] = read.Confidence
	return probabilities
}

// withVotesOnly keeps the counts of a question that has no prediction, so that the
// record shows them.
func withVotesOnly(read QuestionReading, counts map[string]int, k int) QuestionReading {
	for _, count := range counts {
		read.Votes = max(read.Votes, count)
	}
	read.Share = float64(read.Votes) / float64(k)
	return read
}
