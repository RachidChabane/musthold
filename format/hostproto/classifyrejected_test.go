package hostproto_test

import (
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

func TestARejectedCallLeavesItsQuestionWithoutAPrediction(t *testing.T) {
	read := func(mutate func(*hostproto.ClassifyReport)) (hostproto.ClassifyReading, error) {
		report := reportWith(t, violates, violates, violates, violates, violates)
		mutate(&report)
		return hostproto.ReadClassification(classifyRequestID, hostprototest.ClosedSet(), classifySource(t), report, hostprototest.ClassifyTable())
	}
	rejectLast := func(report *hostproto.ClassifyReport) {
		last := &report.Calls[hostproto.ClassifyCalls-1]
		last.Label, last.Attempts, last.Rejected = nil, hostproto.MaxClassifyAttempts, true
	}

	t.Run("four labels and a rejected call keep the votes in the record and close the gate with their own reason [verifies SP-1905~1]", func(t *testing.T) {
		reading, err := read(rejectLast)
		if err != nil {
			t.Fatal(err)
		}
		question := reading.Questions[0]
		if reading.Gate != hostproto.GateRejected || reading.Answers() || question.Predicts() || question.Rejected != 1 || question.Modal != "" {
			t.Fatalf("gate %q question %+v", reading.Gate, question)
		}
		if recorded := reading.Record.Questions[0]; recorded.Votes != 4 || recorded.Share != 0.8 || recorded.Modal != "" {
			t.Fatalf("recorded %+v", recorded)
		}
		if call := reading.Record.Calls[hostproto.ClassifyCalls-1]; !call.Rejected || call.Label != nil {
			t.Fatalf("call %+v", call)
		}
	})

	t.Run("a call that returned no label is a missing vote, which only counts against the share [verifies SP-1905~1]", func(t *testing.T) {
		reading, err := read(func(report *hostproto.ClassifyReport) {
			last := &report.Calls[hostproto.ClassifyCalls-1]
			last.Label, last.Attempts = nil, hostproto.MaxClassifyAttempts
		})
		if err != nil || !reading.Answers() || reading.Questions[0].Votes != 4 || reading.Questions[0].Rejected != 0 {
			t.Fatalf("reading %+v (%v)", reading, err)
		}
	})

	t.Run("a rejected call that also returns a label is refused [verifies SP-1906~1]", func(t *testing.T) {
		if _, err := read(func(report *hostproto.ClassifyReport) { report.Calls[0].Rejected = true }); !errors.Is(err, hostproto.ErrClassifyReport) {
			t.Fatalf("error %v", err)
		}
	})
}
