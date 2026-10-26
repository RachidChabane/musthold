package hostproto_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

// Each fuzz target has a body that states its invariants, a Fuzz function that
// feeds it generated input, and a regular test that feeds it the seed corpus so
// that the invariants are traced and run on every build.

func reportSeeds() []string {
	return []string{
		``, `{}`, `[]`, `null`, `{"calls":null}`, `{"cliVersion":"2.1.293"}`, `{"cliVersion":"v","sessionModel":"m","calls":[{}]}`,
		`{"cliVersion":"v","sessionModel":"m","calls":[{"question":"q1","i":0,"model":"m","label":null,"attempts":2,"rejected":true}]}`,
		`{"cliVersion":"v","sessionModel":"m","calls":[{"question":"q1","i":-1,"model":"","label":7,"attempts":"x"}]}`,
		`{"cliVersion":"v","sessionModel":"m","extra":1,"calls":[]}`,
	}
}

func reportBody(t *testing.T, document []byte) {
	t.Helper()
	report, err := hostproto.ParseClassifyReport(document)
	if err != nil {
		return
	}
	reading, err := hostproto.ReadClassification(classifyRequestID, hostprototest.ClosedSet(), classifySource(t), report, hostprototest.ClassifyTable())
	if err != nil {
		return
	}
	if !reading.Answers() {
		if _, err := reading.Output(hostprototest.ClosedSet()); err == nil {
			t.Fatal("a closed gate rendered an output")
		}
		return
	}
	for _, question := range reading.Questions {
		if !question.Predicts() || question.Confidence < hostproto.TauClassify || question.Rejected > 0 {
			t.Fatalf("the gate is open for the question %+v", question)
		}
	}
	output, err := reading.Output(hostprototest.ClosedSet())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hostproto.ParseClosedAnswers(output); err != nil {
		t.Fatalf("an output that cannot be read back: %v", err)
	}
}

func FuzzClassifyReport(f *testing.F) {
	for _, seed := range reportSeeds() {
		f.Add([]byte(seed))
	}
	f.Add(hostprototest.ClassifyReportOf(f, contractItemForFuzz(), hostprototest.Always("VIOLATES")))
	f.Fuzz(func(t *testing.T, document []byte) { reportBody(t, document) })
}

func TestClassifyReportSeedCorpus(t *testing.T) {
	t.Run("the seeds of the report decoder satisfy its invariants [verifies SP-1906~1]", func(t *testing.T) {
		for _, seed := range reportSeeds() {
			reportBody(t, []byte(seed))
		}
		reportBody(t, hostprototest.ClassifyReportOf(t, contractItemForFuzz(), hostprototest.Always("VIOLATES")))
	})
}

func contractItemForFuzz() hostproto.ClassifyPlanItem {
	set := hostprototest.ClosedSet()
	plan, _ := set.ClassifyPlan()
	source, _ := hostproto.ClassifySourceOf(hostprototest.ClassifySessionModel, "")
	calls, _ := plan.Calls(source)
	return hostproto.ClassifyPlanItem{Calls: calls}
}

func detailSeeds() []string {
	return []string{
		``, `{}`, `null`, `[]`, `{"source":"s"}`, `{"source":"s","cliVersion":"v","record":{}}`, `{"source":"s","importedAt":"not a time"}`,
		`{"verdict":"raised","loggedCalls":5}`, `{"verdict":7}`, `{"verdict":"none","querySources":"x"}`,
	}
}

func detailBody(t *testing.T, document []byte) {
	t.Helper()
	if detail, parsed := hostproto.ParseClassifyDetail(string(document)); parsed {
		if detail.Source == "" {
			t.Fatal("a detail without a source was accepted")
		}
		if _, err := json.Marshal(detail); err != nil {
			t.Fatalf("a detail that cannot be encoded: %v", err)
		}
	}
	if witness, parsed := hostproto.ParseShadowWitness(string(document)); parsed {
		if witness.Verdict == "" {
			t.Fatal("an observation without a verdict was accepted")
		}
		if _, err := json.Marshal(witness); err != nil {
			t.Fatalf("an observation that cannot be encoded: %v", err)
		}
	}
}

func FuzzClassifyJournalDetails(f *testing.F) {
	for _, seed := range detailSeeds() {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, document []byte) { detailBody(t, document) })
}

func TestClassifyJournalDetailSeedCorpus(t *testing.T) {
	t.Run("the seeds of the journal detail decoders satisfy their invariants [verifies SP-1913~1]", func(t *testing.T) {
		for _, seed := range detailSeeds() {
			detailBody(t, []byte(seed))
		}
	})
}

// witnessCaseForFuzz is the classification of the closed fixture the fuzzed log
// is checked against.
func witnessCaseForFuzz(t classifyTester) witnessCase {
	t.Helper()
	return newWitnessCase(t, "VIOLATES", "VIOLATES", "VIOLATES", "VIOLATES", "VIOLATES")
}

func witnessSeeds(t classifyTester) []struct{ index, body string } {
	honest := witnessCaseForFuzz(t).honestLog()
	index := strings.Join(honest.index, "\n")
	return []struct{ index, body string }{
		{``, ``}, {`{}`, `{}`}, {"\n\n", `null`}, {`[1,2]`, `[]`}, {`{"timestamp":1,"query_source":"hook_prompt"}`, `{"messages":7}`},
		{`{"timestamp":"x","query_source":"hook_prompt","request_file":"a","response_file":"b"}`, `{"max_tokens":20,"system":[1],"messages":[{"role":"user","content":[null]}]}`},
		{index, string(honest.files["req_0001.request.json"])},
	}
}

func witnessBody(t *testing.T, index, body []byte) {
	t.Helper()
	w := witnessCaseForFuzz(t)
	builder := w.honestLog()
	files := hostprototest.MemoryFiles{hostproto.WitnessIndexFile: index}
	for name := range builder.files {
		files[name] = body
	}
	for _, anySource := range []bool{false, true} {
		w.input.AnyQuerySource = anySource
		result := hostproto.CheckClassifyWitness(hostproto.FileLog{Files: files}, w.input)
		if !slices.Contains([]hostproto.WitnessVerdict{hostproto.WitnessRaised, hostproto.WitnessExposed, hostproto.WitnessLabelsContradicted, hostproto.WitnessNone}, result.Verdict) {
			t.Fatalf("verdict %q", result.Verdict)
		}
		if result.Verdict != hostproto.WitnessRaised {
			continue
		}
		if len(result.RequestIDs) != hostproto.ClassifyCalls {
			t.Fatalf("a witness of %d entries for %d calls", len(result.RequestIDs), hostproto.ClassifyCalls)
		}
		for _, id := range result.RequestIDs {
			if !strings.Contains(string(index), id) {
				t.Fatalf("the witness names the entry %q, which the index does not hold", id)
			}
		}
	}
}

func FuzzWitnessLog(f *testing.F) {
	for _, seed := range witnessSeeds(f) {
		f.Add([]byte(seed.index), []byte(seed.body))
	}
	f.Fuzz(func(t *testing.T, index, body []byte) { witnessBody(t, index, body) })
}

func TestWitnessLogSeedCorpus(t *testing.T) {
	t.Run("the seeds of the log reader never panic and never witness an entry the index lacks [verifies SP-1913~1]", func(t *testing.T) {
		for _, seed := range witnessSeeds(t) {
			witnessBody(t, []byte(seed.index), []byte(seed.body))
		}
	})
}
