package hostproto_test

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	sessionFixturePath = "testdata/copilot/session-events.jsonl"
	fixtureAgentID     = "a7e0364c-b934-43d8-9bbb-9744dbf2af35"
	fixtureCLIVersion  = "1.0.92"

	fixtureFetchTool  = "musthold-dp-exec-fetch_request"
	fixtureSubmitTool = "musthold-dp-exec-submit_response"
)

func readSessionFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(sessionFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func decodeFixtureRun(t *testing.T, log string, agentID string) hostproto.SessionRun {
	t.Helper()
	run, err := hostproto.DecodeCopilotSessionLog(strings.NewReader(log), agentID)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestDecodeCopilotSessionLog(t *testing.T) {
	t.Run("the run of the agent is read from the session log of the CLI [verifies SP-1870~1]", func(t *testing.T) {
		run := decodeFixtureRun(t, readSessionFixture(t), fixtureAgentID)
		if run.CLIVersion != fixtureCLIVersion {
			t.Errorf("version = %q", run.CLIVersion)
		}
		wantSelected := [][]string{{"musthold-dp-exec/fetch_request", "musthold-dp-exec/submit_response"}}
		if !reflect.DeepEqual(run.Selected, wantSelected) {
			t.Errorf("selected = %v", run.Selected)
		}
		if len(run.UserMessages) != 1 || run.UserMessages[0] != hostproto.ExecutorPromptOf(recordedTicket) {
			t.Errorf("user messages = %q", run.UserMessages)
		}
		if len(run.SystemMessages) != 1 || !strings.HasPrefix(run.SystemMessages[0], hostproto.ExecutorInstructions+hostproto.SystemBlockSeparator) {
			t.Errorf("system messages = %q", run.SystemMessages)
		}
		if len(run.Calls) != 2 || run.Calls[0].Name != fixtureFetchTool || run.Calls[1].Name != fixtureSubmitTool {
			t.Errorf("calls = %+v", run.Calls)
		}
	})

	t.Run("the events of the main agent are not the agent's [verifies SP-1870~1]", func(t *testing.T) {
		run := decodeFixtureRun(t, readSessionFixture(t), fixtureAgentID)
		for _, call := range run.Calls {
			if call.Name == "task" {
				t.Errorf("the launch call of the main agent is in the run: %+v", call)
			}
		}
		other := decodeFixtureRun(t, readSessionFixture(t), "another-agent")
		if len(other.Calls)+len(other.Selected)+len(other.UserMessages)+len(other.SystemMessages) != 0 || other.CLIVersion != fixtureCLIVersion {
			t.Errorf("another agent's run = %+v", other)
		}
	})

	t.Run("the bookkeeping call of the CLI is not an action of the agent [verifies SP-1870~1]", func(t *testing.T) {
		log := `{"type":"tool.execution_start","agentId":"a","data":{"toolName":"report_intent","arguments":{"intent":"x"}}}` + "\n" +
			`{"type":"tool.execution_start","agentId":"a","data":{"toolName":"bash","arguments":"ls"}}` + "\n"
		run := decodeFixtureRun(t, log, "a")
		if len(run.Calls) != 1 || run.Calls[0].Name != "bash" {
			t.Errorf("calls = %+v", run.Calls)
		}
	})

	t.Run("an incomplete last line is ignored and an unreadable line elsewhere is refused [verifies SP-1870~1]", func(t *testing.T) {
		log := readSessionFixture(t)
		decodeFixtureRun(t, log+`{"type":"assistant.mes`, fixtureAgentID)
		_, err := hostproto.DecodeCopilotSessionLog(strings.NewReader(`{"type":`+"\n"+log), fixtureAgentID)
		if !errors.Is(err, hostproto.ErrBadTranscript) {
			t.Errorf("an unreadable first line: %v", err)
		}
	})

	t.Run("a read for no agent is refused [verifies SP-1870~1]", func(t *testing.T) {
		if _, err := hostproto.DecodeCopilotSessionLog(strings.NewReader(readSessionFixture(t)), ""); !errors.Is(err, hostproto.ErrNoAgentID) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("a selection event with no tools is an empty selection [verifies SP-1870~1]", func(t *testing.T) {
		run := decodeFixtureRun(t, `{"type":"subagent.selected","agentId":"a","data":{}}`+"\n", "a")
		if len(run.Selected) != 1 || run.Selected[0] == nil || len(run.Selected[0]) != 0 {
			t.Errorf("selected = %#v", run.Selected)
		}
	})
}

func TestSubmittedOutput(t *testing.T) {
	t.Run("the output of a submission is read as an object or as the string that holds it [verifies SP-1870~1]", func(t *testing.T) {
		for name, input := range map[string]string{
			"object": `{"ticket":"t","output":{"validity":"VALID"}}`,
			"string": `{"ticket":"t","output":"{\"validity\":\"VALID\"}"}`,
		} {
			output, found := hostproto.SubmittedOutput(hostproto.ToolCall{Name: "x", Input: []byte(input)})
			if !found || string(output) != `{"validity":"VALID"}` {
				t.Errorf("%s: %q, %v", name, output, found)
			}
		}
		if _, found := hostproto.SubmittedOutput(hostproto.ToolCall{Input: []byte(`{"ticket":"t"}`)}); found {
			t.Error("a call with no output yielded one")
		}
	})
}

func sessionLogBody(t *testing.T, log []byte) {
	t.Helper()
	first, err := hostproto.DecodeCopilotSessionLog(strings.NewReader(string(log)), fixtureAgentID)
	if err != nil {
		return
	}
	for _, call := range first.Calls {
		if call.Name == hostproto.CopilotIntentTool {
			t.Fatalf("the bookkeeping call is in the run: %+v", call)
		}
	}
	again, err := hostproto.DecodeCopilotSessionLog(strings.NewReader(string(log)), fixtureAgentID)
	if err != nil || !reflect.DeepEqual(first, again) {
		t.Fatalf("the decoding is not deterministic: %+v then %+v (%v)", first, again, err)
	}
}

func sessionLogSeeds(t *testing.T) []string {
	t.Helper()
	return []string{
		``, "\n\n", `{}`, `[]`, `{"type":"subagent.selected","agentId":"` + fixtureAgentID + `","data":{"tools":[1]}}`,
		`{"type":"tool.execution_start","agentId":"` + fixtureAgentID + `","data":{"toolName":"report_intent"}}`,
		readSessionFixture(t), "{\"type\":\"session.start\"}\n{",
	}
}

func FuzzCopilotSessionLog(f *testing.F) {
	raw, err := os.ReadFile(sessionFixturePath)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(raw)
	for _, seed := range []string{``, "\n\n", `{}`, `{"type":"subagent.selected","agentId":"x","data":{"tools":[1]}}`, "{\"type\":\"session.start\"}\n{"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, log []byte) { sessionLogBody(t, log) })
}

func TestSessionLogSeedCorpus(t *testing.T) {
	t.Run("the session log decoder holds its invariants on the seed corpus [verifies SP-1870~1]", func(t *testing.T) {
		for _, seed := range sessionLogSeeds(t) {
			sessionLogBody(t, []byte(seed))
		}
	})
}
