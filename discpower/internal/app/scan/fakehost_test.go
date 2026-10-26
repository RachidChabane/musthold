package scan_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

// Answers of the fake host.
const (
	fixedConfidence = 0.9
	firstSlot       = power.SlotDirect
)

// scopeLabel finds the witnesses a scope request shows.
var scopeLabel = regexp.MustCompile(`Witness (w\d+), for the clause`)

// answerFunc answers one ticket: the request, the vote and how many times
// that vote was answered before.
type answerFunc func(request hostproto.Request, vote, times int) string

// fakeHost answers the emitted tickets of a session honestly, unless a test
// overrides how a kind is answered.
type fakeHost struct {
	store    hostproto.HostStore
	clock    *clock
	promises []promiseAnswer
	answers  map[hostproto.Kind]answerFunc
	times    map[hostproto.Key]int
}

// promiseAnswer is one promise the fake host extracts.
type promiseAnswer struct {
	Quote  string `json:"quote"`
	Source string `json:"source"`
}

func newFakeHost(store hostproto.HostStore, clock *clock) *fakeHost {
	return &fakeHost{
		store: store, clock: clock, answers: map[hostproto.Kind]answerFunc{}, times: map[hostproto.Key]int{},
		promises: []promiseAnswer{{Quote: quoteZero, Source: format.ScanSourceReadme}},
	}
}

// answer answers every emitted ticket of the session and counts them.
func (h *fakeHost) answer(t *testing.T, session hostproto.Session) int {
	t.Helper()
	entries, err := h.store.ListEntries()
	if err != nil {
		t.Fatal(err)
	}
	index := sessionIndex(t, h.store)
	answered := 0
	for _, entry := range entries {
		if entry.State != hostproto.StateEmitted || !index.Selects(hostproto.OnlySession(session), entry.Ticket.RequestID) {
			continue
		}
		request, err := h.store.GetRequest(entry.Ticket.RequestID)
		if err != nil {
			t.Fatal(err)
		}
		key := entry.Ticket.Key()
		output := h.output(request, key.Vote, h.times[key])
		h.times[key]++
		h.submit(t, entry, output)
		answered++
	}
	return answered
}

func sessionIndex(t *testing.T, store hostproto.HostStore) hostproto.SessionIndex {
	t.Helper()
	items, err := store.Items()
	if err != nil {
		t.Fatal(err)
	}
	return hostproto.IndexSessions(items)
}

// submit fetches the ticket and accepts the output as its answer.
func (h *fakeHost) submit(t *testing.T, entry hostproto.Entry, output string) {
	t.Helper()
	key := entry.Ticket.Key()
	if _, err := h.store.UpdateEntry(key, func(e hostproto.Entry) (hostproto.Entry, error) { return e.Fetch("", h.clock.peek()) }); err != nil {
		t.Fatal(err)
	}
	digest, err := hostproto.OutputDigest([]byte(output))
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := h.store.UpdateEntry(key, func(e hostproto.Entry) (hostproto.Entry, error) { return e.Accept(digest) })
	if err != nil {
		t.Fatal(err)
	}
	response := hostprototest.Response(accepted.Ticket)
	response.Output = []byte(output)
	if err := h.store.PutResponse(response); err != nil {
		t.Fatal(err)
	}
}

func (h *fakeHost) output(request hostproto.Request, vote, times int) string {
	if answer, overridden := h.answers[request.Kind]; overridden {
		return answer(request, vote, times)
	}
	switch request.Kind {
	case hostproto.KindGeneratePromises:
		return must(map[string]any{"promises": h.promises})
	case hostproto.KindGenerateWitness:
		return honestWitnesses(vote)
	case hostproto.KindJudgeScope:
		return honestScope(request)
	case hostproto.KindGenerateTargeted:
		return honestGeneration(breaksTotal)
	case hostproto.KindJudgeLabels:
		return must(map[string]any{"labels": map[string][]string{"p1": {"f1"}}})
	}
	return "{}"
}

// witnessCode is a witness that requires a text of the source.
func witnessCode(name, required string) string {
	return "def " + name + "():\n    # requires: " + required + "\n    assert discount(10, 0) == 10\n"
}

// honestWitnesses are three examples and one property, named after the vote
// so that the two writers write distinct witnesses: the first requires the
// line the model mutant breaks.
func honestWitnesses(vote int) string {
	type item struct {
		Name   string       `json:"name"`
		Clause string       `json:"clause"`
		Kind   witness.Kind `json:"kind"`
		Code   string       `json:"code"`
	}
	prefix := "test_v" + string(rune('a'+vote)) + "_"
	items := []item{
		{prefix + "zero", "zero percent", witness.KindExample, witnessCode(prefix+"zero", keepsTotal)},
		{prefix + "guard", "zero percent", witness.KindExample, witnessCode(prefix+"guard", "    if percent <= 0:")},
		{prefix + "free", "free shipping", witness.KindExample, witnessCode(prefix+"free", "    if total >= 50:")},
		{prefix + "any", "any total", witness.KindProperty, witnessCode(prefix+"any", "def discount(total: int, percent: int) -> int:")},
	}
	return must(map[string]any{"imports": witnessImports, "witnesses": items})
}

// honestScope holds every witness the request shows fixed.
func honestScope(request hostproto.Request) string {
	verdicts := map[string]any{}
	for _, match := range scopeLabel.FindAllStringSubmatch(request.Prompt.User, -1) {
		verdicts[match[1]] = map[string]any{"answer": witness.ScopeFixed, "confidence": fixedConfidence}
	}
	return must(map[string]any{"verdicts": verdicts})
}

// honestGeneration proposes one mutant of discount whose third line becomes
// the given line.
func honestGeneration(line string) string {
	source := strings.Split(totalsSource, "\n\n\n")[0]
	source = strings.Replace(source, keepsTotal+"\n", line+"\n", 1)
	return must(map[string]any{"mutants": []any{map[string]any{
		"slot": firstSlot, "mechanism": "change the total kept at zero percent",
		"edits": []any{map[string]any{"file": totals, "qualname": "discount", "new_source": source}},
	}}})
}
