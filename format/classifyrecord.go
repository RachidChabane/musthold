package format

import (
	"fmt"
	"slices"
	"strings"
)

// ClassifyCallRecord is one classifier call of a classification as a decision
// records it: the question, the index of the call, the model that answered, the
// labels in the order the call listed them and the label that came back, nil for
// a missing vote.
type ClassifyCallRecord struct {
	Question string   `json:"question"`
	Index    int      `json:"i"`
	Model    string   `json:"model"`
	Order    []string `json:"order"`
	Label    *string  `json:"label"`
	Attempts int      `json:"attempts"`
}

// ClassifyQuestionRecord is what the calls of one question settled: the seed
// input and seed that ordered its labels, the modal label with its count and
// share over all calls, whether the top counts tied, and the confidence the
// table gave the count.
type ClassifyQuestionRecord struct {
	Question   string  `json:"question"`
	SeedInput  string  `json:"seedInput"`
	Seed       string  `json:"seed"`
	Modal      string  `json:"modal,omitempty"`
	Votes      int     `json:"votes"`
	Share      float64 `json:"share"`
	Tied       bool    `json:"tied,omitempty"`
	Confidence float64 `json:"confidence"`
}

// ClassifyRecord is the classification of one closed request: where the
// confidence came from, the version of the host that made the calls, the digest
// of the question set and every question and call, so that the plan and the
// answer can be recomputed from the record.
type ClassifyRecord struct {
	RequestID       string                   `json:"requestId"`
	Source          ConfidenceSource         `json:"source"`
	Mapping         string                   `json:"mapping"`
	CLIVersion      string                   `json:"cliVersion"`
	SessionModel    string                   `json:"sessionModel"`
	ClosedSetDigest string                   `json:"closedSetDigest"`
	Questions       []ClassifyQuestionRecord `json:"questions"`
	Calls           []ClassifyCallRecord     `json:"calls"`
}

// Rules a record must satisfy to be part of a decision.
func (r ClassifyRecord) check(sink *collector, path string) {
	checkDigest(sink, path+".requestId", r.RequestID)
	checkDigest(sink, path+".closedSetDigest", r.ClosedSetDigest)
	spec, err := ParseClassifySource(string(r.Source))
	if err != nil {
		sink.add(CodeUnknownValue, path+".source", "%q is not a host-classify source", r.Source)
	}
	if _, err := ParseHostClassifyMapping(r.Mapping); err != nil {
		sink.add(CodeUnknownValue, path+".mapping", "%q is not a host-classify mapping", r.Mapping)
	}
	checkNotEmpty(sink, path+".cliVersion", r.CLIVersion)
	checkNotEmpty(sink, path+".sessionModel", r.SessionModel)
	if err == nil && len(r.Questions)*spec.K != len(r.Calls) {
		sink.add(CodeUnknownValue, path+".calls", "%d calls for %d questions of %d calls", len(r.Calls), len(r.Questions), spec.K)
	}
	for index, question := range r.Questions {
		question.check(sink, fmt.Sprintf("%s.questions[%d]", path, index))
	}
	for index, call := range r.Calls {
		call.check(sink, fmt.Sprintf("%s.calls[%d]", path, index))
	}
}

func (q ClassifyQuestionRecord) check(sink *collector, path string) {
	checkNotEmpty(sink, path+".question", q.Question)
	checkNotEmpty(sink, path+".seedInput", q.SeedInput)
	if !strings.HasSuffix(q.SeedInput, "|"+q.Question) {
		sink.add(CodeUnknownValue, path+".seedInput", "%q does not end with the question %q", q.SeedInput, q.Question)
	}
	checkConfidence(sink, path+".share", q.Share)
	checkConfidence(sink, path+".confidence", q.Confidence)
}

func (c ClassifyCallRecord) check(sink *collector, path string) {
	checkNotEmpty(sink, path+".question", c.Question)
	checkNotEmpty(sink, path+".model", c.Model)
	if c.Label != nil && !slices.Contains(c.Order, *c.Label) {
		sink.add(CodeUnknownValue, path+".label", "%q is not one of the labels %v of the call", *c.Label, c.Order)
	}
}
