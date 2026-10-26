package hostproto_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

func journalOf(version string, kinds ...hostproto.EventKind) []hostproto.Record {
	var records []hostproto.Record
	for _, kind := range kinds {
		detail := hostproto.ClassifyJournalDetail{Source: "host-classify(5,rot1):m*5", CLIVersion: version}
		event := hostproto.Event{Kind: kind}
		if kind == hostproto.EventClassifyAnswered || kind == hostproto.EventClassifyShadow || kind == hostproto.EventClassifyFellThrough {
			event.Detail = hostproto.ClassifyDetail(hostproto.ClassifyReading{Record: format.ClassifyRecord{Source: format.ConfidenceSource(detail.Source), CLIVersion: version}}, hostprototest.Epoch)
		}
		records = append(records, hostproto.Record{Event: event})
	}
	return records
}

func TestDiagnoseClassification(t *testing.T) {
	admission := hostprototest.ClassifyAdmissionOf(t, format.KindAlignment, classifySource(t), hostprototest.ClassifySessionModel)

	t.Run("a mismatch between the host that made the latest classification and an admitted entry fails [verifies SP-1908~1]", func(t *testing.T) {
		diagnosis := hostproto.DiagnoseClassification(hostproto.ClassifyAuto, admission, format.KindAlignment, journalOf("2.1.400", hostproto.EventClassifyAnswered))
		if diagnosis.Passed || !strings.Contains(diagnosis.Detail, "host version mismatch") || !strings.Contains(diagnosis.Detail, "2.1.293") {
			t.Fatalf("diagnosis %+v", diagnosis)
		}
	})

	t.Run("the same host version passes, and the latest classification decides [verifies SP-1908~1]", func(t *testing.T) {
		records := append(journalOf("2.1.400", hostproto.EventClassifyShadow), journalOf(hostprototest.ClassifyCLIVersion, hostproto.EventClassifyFellThrough)...)
		diagnosis := hostproto.DiagnoseClassification(hostproto.ClassifyAuto, admission, format.KindAlignment, records)
		if !diagnosis.Passed || !strings.Contains(diagnosis.Detail, "mode auto, 1 admitted, host "+hostprototest.ClassifyCLIVersion) {
			t.Fatalf("diagnosis %+v", diagnosis)
		}
	})

	t.Run("an entry of another module is not this module's to report, and no classification yet is no mismatch [verifies SP-1908~1]", func(t *testing.T) {
		other := hostproto.DiagnoseClassification(hostproto.ClassifyAuto, admission, format.KindCompleteness, journalOf("2.1.400", hostproto.EventClassifyAnswered))
		none := hostproto.DiagnoseClassification(hostproto.ClassifyShadow, admission, format.KindAlignment, nil)
		if !other.Passed || !strings.Contains(other.Detail, "0 admitted") || !none.Passed || !strings.Contains(none.Detail, "mode shadow, 1 admitted") {
			t.Fatalf("other %+v none %+v", other, none)
		}
	})
}
