package scan_test

import (
	"encoding/json"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
)

const verifiesUnitText = " [verifies SP-2281~1]"

// endingInLineBreaks is a generation answer whose sources end with the line
// breaks a model ends its code with.
func endingInLineBreaks(t *testing.T, answer string) string {
	t.Helper()
	var document map[string][]map[string]any
	if err := json.Unmarshal([]byte(answer), &document); err != nil {
		t.Fatal(err)
	}
	for _, mutant := range document["mutants"] {
		for _, edit := range mutant["edits"].([]any) {
			edit.(map[string]any)["new_source"] = edit.(map[string]any)["new_source"].(string) + "\n\n"
		}
	}
	return must(document)
}

func TestModelMutantsEndingInLineBreaks(t *testing.T) {
	f := newHostFixture()
	f.host.answers[hostproto.KindGenerateTargeted] = func(hostproto.Request, int, int) string {
		return endingInLineBreaks(t, honestGeneration(breaksTotal))
	}
	results := f.loop(t, hostRequest(), 6)
	final := results[len(results)-1]
	t.Run("a generation answer whose sources end in line breaks is usable: it is never asked again and its mutant joins the draw"+verifiesUnitText, func(t *testing.T) {
		for key, times := range f.host.times {
			request, err := f.store.GetRequest(key.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			if request.Kind == hostproto.KindGenerateTargeted && times > 1 {
				t.Errorf("generation request %s answered %d times", key.RequestID, times)
			}
		}
		if final.Report.Promises.ModelMutants == 0 {
			t.Fatal("no model mutant joined the draw")
		}
	})
}
