package hostprototest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// ImportedClassification is what one import of the mod printed.
type ImportedClassification struct {
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
	Ticket  string `json:"ticket"`
}

// CommandRunner runs the command line of a module with a standard input and
// returns its exit code, standard output and standard error.
type CommandRunner func(stdin []byte, args ...string) (code int, stdout, stderr string)

// RawAPILog writes the raw API log the user's host keeps: an index and a request
// and a response body for each classifier call.
type RawAPILog struct {
	t     testing.TB
	dir   string
	at    time.Time
	model string
	next  int
}

// ClassifierModel is the model the fake host's log shows answering.
const ClassifierModel = "claude-sonnet-5-5-20260101"

// NewRawAPILog opens an empty log in the directory; every call it records is
// stamped with the time.
func NewRawAPILog(t testing.TB, dir string, at time.Time) *RawAPILog {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	return &RawAPILog{t: t, dir: dir, at: at, model: ClassifierModel}
}

// Record logs the calls of a plan as the host sends them, each answered with the
// label the function returns.
func (l *RawAPILog) Record(item hostproto.ClassifyPlanItem, answer func(hostproto.ClassifiedCall) string) {
	l.t.Helper()
	var lines []string
	for _, call := range item.Calls {
		l.next++
		id := fmt.Sprintf("req_%06d", l.next)
		request, _ := json.Marshal(map[string]any{
			"model": l.model, "max_tokens": hostproto.ClassifyMaxTokens, "system": hostproto.ClassifierSystemPrompt(call.Labels),
			"messages": []map[string]any{{"role": "user", "content": hostproto.ClassifierUserPrompt(call.Text)}},
		})
		response, _ := json.Marshal(map[string]any{"content": []map[string]any{{"type": "text", "text": answer(call)}}})
		l.write(id+".request.json", request)
		l.write(id+".response.json", response)
		line, _ := json.Marshal(map[string]any{
			"timestamp": l.at.Format(time.RFC3339Nano), "session_id": "session-1", "query_source": hostproto.ClassifyQuerySource, "model": l.model,
			"request_id": id, "request_file": id + ".request.json", "response_file": id + ".response.json",
		})
		lines = append(lines, string(line))
	}
	index, err := os.OpenFile(filepath.Join(l.dir, hostproto.WitnessIndexFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		l.t.Fatal(err)
	}
	defer index.Close()
	if _, err := index.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		l.t.Fatal(err)
	}
}

func (l *RawAPILog) write(name string, content []byte) {
	l.t.Helper()
	if err := os.WriteFile(filepath.Join(l.dir, name), content, 0o600); err != nil {
		l.t.Fatal(err)
	}
}

// ClassifyMod plays the host mod against the command line of a module.
type ClassifyMod struct {
	Run   CommandRunner
	Model string
	// Log, when set, records the calls of every classification that was imported.
	Log *RawAPILog
	// LoggedAs, when set, changes the label the log shows for a call whose mod
	// reported the label given: a mod that reports other labels than the model
	// returned.
	LoggedAs func(call hostproto.ClassifiedCall, reported string) string
}

// Play asks `systemone classify-plan` for the calls, answers every call of a
// question with the label the picker names for it (none for an empty label), and
// pipes the report into `systemone import`. It returns what each import printed.
func (m ClassifyMod) Play(t testing.TB, pick func(item hostproto.ClassifyPlanItem, question string) string) []ImportedClassification {
	t.Helper()
	code, stdout, stderr := m.Run(nil, "systemone", "classify-plan", "--model", m.Model)
	if code != 0 {
		t.Fatalf("classify-plan exited %d: %s", code, stderr)
	}
	var imported []ImportedClassification
	for _, item := range DecodeClassifyPlans(t, stdout) {
		answer := func(call hostproto.ClassifiedCall) string { return pick(item, call.Question) }
		if m.Log != nil {
			m.Log.Record(item, func(call hostproto.ClassifiedCall) string {
				if m.LoggedAs != nil {
					return m.LoggedAs(call, answer(call))
				}
				return answer(call)
			})
		}
		report := ClassifyReportOf(t, item, answer)
		code, printed, stderr := m.Run(report, "systemone", "import", item.Ticket, "--nonce", item.Nonce, "--source", item.Source)
		if code != 0 {
			t.Fatalf("import of %s exited %d: %s", item.Ticket, code, stderr)
		}
		var outcome ImportedClassification
		if err := json.Unmarshal([]byte(printed), &outcome); err != nil {
			t.Fatalf("import printed %q: %v", printed, err)
		}
		imported = append(imported, outcome)
	}
	return imported
}

// ClassifyAsTheMod plays the host mod without a raw API log.
func ClassifyAsTheMod(t testing.TB, run CommandRunner, model string, pick func(item hostproto.ClassifyPlanItem, question string) string) []ImportedClassification {
	t.Helper()
	return ClassifyMod{Run: run, Model: model}.Play(t, pick)
}
