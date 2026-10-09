package format_test

import (
	"reflect"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/proofs"
)

func classifyRecord(requestID string) format.ClassifyRecord {
	source, _ := format.SingleVoterClassifySource(5, format.ClassifySchemeRot1, sonnet)
	label := "VIOLATES"
	record := format.ClassifyRecord{
		RequestID: requestID, Source: source.Source(), Mapping: format.HostClassifyMapping(proofs.Digest("table")),
		CLIVersion: "2.1.293", SessionModel: sonnet, ClosedSetDigest: proofs.Digest("set"),
		Questions: []format.ClassifyQuestionRecord{{
			Question: "q1", SeedInput: "host-classify|rot1|" + proofs.Digest("set") + "|q1", Seed: "923c677823c6fceba0508f3994ae1910",
			Modal: label, Votes: 5, Share: 1, Confidence: 0.99,
		}},
	}
	for call := range 5 {
		record.Calls = append(record.Calls, format.ClassifyCallRecord{Question: "q1", Index: call, Model: sonnet, Order: []string{"VIOLATES", "CONSISTENT"}, Label: &label, Attempts: 1})
	}
	return record
}

func decisionWith(records ...format.ClassifyRecord) *format.Decision {
	source, _ := format.SingleVoterClassifySource(5, format.ClassifySchemeRot1, sonnet)
	return &format.Decision{
		Basis: format.BasisSystemOne, Deciders: []string{sonnet}, ConfidenceSource: source.Source(),
		ConfidenceMapping: format.HostClassifyMapping(proofs.Digest("table")), RawConfidence: 0.99, Threshold: 0.7, Classify: records,
	}
}

func TestClassificationInADecision(t *testing.T) {
	first, second := proofs.Digest("request-a"), proofs.Digest("request-b")
	if first > second {
		first, second = second, first
	}

	t.Run("a decision that names a classification source and records its classifications validates and survives an encode and decode [verifies SP-1903~1]", func(t *testing.T) {
		original := proofs.Alignment(func(s *alignment) {
			s.Predicate.Extensions.Decision = decisionWith(classifyRecord(first), classifyRecord(second))
		})
		if err := original.Validate(); err != nil {
			t.Fatal(err)
		}
		decoded, err := format.DecodeAs[format.Alignment](mustEncode(t, original))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded.Predicate.Extensions.Decision, original.Predicate.Extensions.Decision) {
			t.Fatalf("the decision changed:\n%+v\n%+v", decoded.Predicate.Extensions.Decision, original.Predicate.Extensions.Decision)
		}
	})

	t.Run("a classification record that breaks its rules is reported [verifies SP-1903~1]", func(t *testing.T) {
		cases := []struct {
			name string
			edit func(*format.Decision)
			want format.ViolationCode
		}{
			{"records out of order", func(d *format.Decision) {
				d.Classify = []format.ClassifyRecord{classifyRecord(second), classifyRecord(first)}
			}, format.CodeUnsorted},
			{"a request id that is no digest", func(d *format.Decision) { d.Classify[0].RequestID = "abc" }, format.CodeBadDigest},
			{"a source of another kind", func(d *format.Decision) { d.Classify[0].Source = format.HostVotesSource(3) }, format.CodeUnknownValue},
			{"a mapping that names no table", func(d *format.Decision) { d.Classify[0].Mapping = format.MappingModalShare }, format.CodeUnknownValue},
			{"no host version", func(d *format.Decision) { d.Classify[0].CLIVersion = "" }, format.CodeMissingField},
			{"a call count that is not k per question", func(d *format.Decision) { d.Classify[0].Calls = d.Classify[0].Calls[:4] }, format.CodeUnknownValue},
			{"a label outside the call's order", func(d *format.Decision) {
				other := "MAYBE"
				d.Classify[0].Calls[0].Label = &other
			}, format.CodeUnknownValue},
			{"a share above one", func(d *format.Decision) { d.Classify[0].Questions[0].Share = 1.5 }, format.CodeConfidenceRange},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				statement := proofs.Alignment(func(s *alignment) {
					s.Predicate.Extensions.Decision = decisionWith(classifyRecord(first))
					c.edit(s.Predicate.Extensions.Decision)
				})
				assertCodes(t, statement.Validate(), c.want)
			})
		}
	})
}
