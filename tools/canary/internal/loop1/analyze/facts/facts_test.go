package facts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// recording is the agent-loop demonstration's recorded run: three end-of-turn
// calls, a pending block at 21:00:22, a finding block on BOX-2 at 21:04:00, an
// edit of tests/test_prices.py at 21:04:26 and a pass at 21:09:21.
const recording = "../../../demoloop/app/testdata/recording"

const project = "<run>/project"

func finding(code, criterion, module string) record.GateFinding {
	return record.GateFinding{Category: code, Blocking: true, RequirementID: criterion, Module: module}
}

func report(findings ...record.GateFinding) *Report {
	r := &Report{}
	r.Verdict, r.ExitCode, r.Findings = "fail", 1, findings
	return r
}

func passing() *Report {
	r := &Report{}
	r.Verdict = passVerdict
	return r
}

func withTests(r *Report, criterion string, files ...string) *Report {
	requirement := Requirement{ID: criterion}
	for _, file := range files {
		var test struct {
			Name   string `json:"name"`
			Source struct {
				File string `json:"file"`
			} `json:"source"`
		}
		test.Source.File = file
		requirement.Tests = append(requirement.Tests, test)
	}
	r.Requirements = append(r.Requirements, requirement)
	return r
}

func matrix(killed, total int) study.KillMatrix {
	m := study.KillMatrix{Killed: map[string]bool{}}
	for index := range total {
		m.Killed[study.StopDir(index)] = index < killed
	}
	return m
}

// writeRecords copies the recording into a records directory and adds the
// stop reports and the analysis files.
func writeRecords(t *testing.T, detection study.Detection, heldOut study.HeldOut) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{record.MetaFile, record.StreamFile, record.TimesFile, record.HooksFile, record.FinalGateFile} {
		data, err := os.ReadFile(filepath.Join(recording, name))
		if err != nil {
			t.Fatal(err)
		}
		if name == record.MetaFile {
			data = []byte(strings.Replace(string(data), `"exitCode": 0`, `"exitCode": 0, "attempt": 2, "outcome": "completed"`, 1))
		}
		writeFile(t, filepath.Join(dir, name), data)
	}
	stops := map[int]*Report{
		1: report(finding(staleProof, "BOX-1", "musthold-alignment"), finding("uncovered_requirement", "BOX-4", "")),
		2: withTests(report(finding("surviving_valid_mutant", "BOX-2", "musthold-dp"), finding(staleProof, "BOX-3", "musthold-dp")), "BOX-2", "tests/test_prices.py"),
		3: passing(),
	}
	for n, r := range stops {
		writeJSON(t, filepath.Join(dir, study.StopsDir, strconv.Itoa(n), study.StopReportFile), r)
	}
	writeJSON(t, filepath.Join(dir, study.OracleFile), study.Oracle{Defective: true, Violated: []string{"BOX-3"}, Flaky: []string{"oracle/test_box1.py::test_a"}})
	writeJSON(t, filepath.Join(dir, study.HeldOutFile), heldOut)
	writeJSON(t, filepath.Join(dir, study.DetectorsFile), detection)
	return dir
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, data)
}

var heldOut = study.HeldOut{
	study.DirEquipped: matrix(2, 4),
	study.DirFinal:    {Killed: matrix(3, 4).Killed, Contradicting: []string{"tests/test_prices.py::test_wrong"}},
	study.StopDir(2):  matrix(2, 4),
	study.StopDir(3):  matrix(3, 4),
}

func loadRecording(t *testing.T, detection study.Detection, held study.HeldOut) Facts {
	t.Helper()
	in, digests, err := Load(writeRecords(t, detection, held), project)
	if err != nil {
		t.Fatal(err)
	}
	if digests[record.StreamFile] == "" || digests["stops/2/report.json"] == "" || digests[study.DetectorsFile] == "" {
		t.Fatalf("digests %v", digests)
	}
	return Compute("boxoffice-treatment-1", in)
}

func TestAcceptance(t *testing.T) {
	t.Run("the change is accepted by gate-after.json, defective by oracle.json, and escapes when both [verifies SP-2587~1]", func(t *testing.T) {
		got := loadRecording(t, study.Detection{}, heldOut)
		if !got.Accepted || !got.Defective || !got.Escaped || got.Attempt != 2 || got.Outcome != study.OutcomeCompleted ||
			strings.Join(got.Violated, ",") != "BOX-3" || len(got.Flaky) != 1 {
			t.Fatalf("got %+v", got)
		}
		in := Input{FinalGate: report(), Oracle: study.Oracle{Defective: true}}
		if f := Compute("r", in); f.Accepted || f.Escaped || !f.Defective {
			t.Fatalf("a blocked final gate is not accepted: %+v", f)
		}
		if f := Compute("r", Input{FinalGate: passing()}); !f.Accepted || f.Escaped {
			t.Fatalf("a clean accepted change does not escape: %+v", f)
		}
	})
}

func TestBlocks(t *testing.T) {
	t.Run("each end-of-turn call is numbered and each block takes its kind [verifies SP-2588~1]", func(t *testing.T) {
		got := loadRecording(t, study.Detection{}, heldOut)
		if len(got.Calls) != 3 || got.Calls[0].Decision != DecisionBlock || got.Calls[2].Decision != DecisionPass || got.Calls[1].HooksLine != 4 {
			t.Fatalf("calls %+v", got.Calls)
		}
		if len(got.Blocks) != 2 || got.Blocks[0].Kind != KindPending || got.Blocks[1].Kind != KindFinding {
			t.Fatalf("blocks %+v", got.Blocks)
		}
		if got.BlockCounts[KindPending] != 1 || got.BlockCounts[KindFinding] != 1 || got.GiveUps != 0 {
			t.Fatalf("counts %+v", got.BlockCounts)
		}
	})
	t.Run("give-ups, other exits, traceability blocks and missing reports [verifies SP-2588~1]", func(t *testing.T) {
		hooks := []evidence.HookCall{
			stop(2, "", "2026-11-03T21:00:00Z"),
			stop(1, GiveUpMessage+" 3 attempts; run `shallnot gate` to see why.", "2026-11-03T21:01:00Z"),
			stop(1, "shallnot: tool failure", "2026-11-03T21:02:00Z"),
			stop(2, "", "2026-11-03T21:03:00Z"),
			{Module: "musthold-dp", Args: []string{"hook", "pre-tool"}, ExitCode: 2},
		}
		in := Input{Hooks: hooks, Stops: map[int]*Report{1: report(finding("uncovered_requirement", "A-1", ""))}}
		got := Compute("r", in)
		if got.GiveUps != 1 || got.Calls[2].Decision != DecisionOther || len(got.Calls) != 4 {
			t.Fatalf("calls %+v", got.Calls)
		}
		if got.Blocks[0].Kind != KindTraceability || got.Blocks[0].ReportMissing || got.Blocks[1].Kind != KindTraceability || !got.Blocks[1].ReportMissing {
			t.Fatalf("blocks %+v", got.Blocks)
		}
		pending, findings := classify(report(finding("awaiting_host", "A-1", "musthold-dp"), finding("uncovered_requirement", "A-2", "")))
		if pending != KindPending || len(findings) != 2 {
			t.Fatalf("got %s %+v", pending, findings)
		}
	})
	t.Run("a finding block lists its proof verdicts by code and criterion [verifies SP-2589~1]", func(t *testing.T) {
		got := loadRecording(t, study.Detection{}, heldOut)
		block := got.Blocks[1]
		if len(block.Findings) != 1 || block.Findings[0].Code != "surviving_valid_mutant" || block.Findings[0].Criterion != "BOX-2" {
			t.Fatalf("findings %+v", block.Findings)
		}
		if len(got.FindingCodes) != 1 || got.FindingCodes["surviving_valid_mutant"] != 1 {
			t.Fatalf("codes %+v", got.FindingCodes)
		}
	})
}

func stop(exit int, stderr, started string) evidence.HookCall {
	return evidence.HookCall{Module: shallnotModule, Args: []string{hookSubcommand, stopHookCommand}, ExitCode: exit, Stderr: stderr, StartedAt: started, DurationMillis: 1000}
}

func TestCorrection(t *testing.T) {
	t.Run("edits are placed between the calls by their arrival time [verifies SP-2590~1]", func(t *testing.T) {
		got := loadRecording(t, study.Detection{}, heldOut)
		// The edits of prices.py and test_prices.py arrive at 21:00:09.8 and
		// 21:00:15.2, before the first call; the edit of the BOX-2 test at 21:04:26,
		// between the second and the third.
		if got.Blocks[0].Edits != 0 || got.Blocks[1].Edits != 1 {
			t.Fatalf("blocks %+v", got.Blocks)
		}
		start := time.Date(2026, 11, 3, 21, 0, 0, 0, time.UTC)
		in := Input{Run: study.Run{}, Project: "/work/runs/a-1/project",
			Hooks: []evidence.HookCall{stop(2, "", "2026-11-03T21:00:10Z"), stop(0, "", "2026-11-03T21:00:30Z")},
			Stream: lines(
				toolUse("Edit", "file_path", "/work/runs/a-1/project/tests/test_a.py", ""),
				toolUse("Write", "file_path", "/work/runs/a-1/project/pkg/mod.py", "toolu_main"),
				toolUse("Edit", "file_path", "/work/runs/a-1/project/README.md", ""),
				toolUse("Edit", "file_path", "/elsewhere/tests/test_a.py", ""),
				toolUse("NotebookEdit", "notebook_path", "tests/nb.py", ""),
				toolUse("Read", "file_path", "/work/runs/a-1/project/tests/test_a.py", ""),
			),
			Arrivals: []record.Arrival{{Line: 1, Millis: 12000}, {Line: 2, Millis: 15000}, {Line: 5, Millis: 31000}},
		}
		in.Run.StartedAt = start
		edits := placedEdits(in)
		// Lines 1, 2 and 5 are source edits: a test, a Python file written by a
		// subagent, and a relative notebook path; line 3 is not source, line 4 lies
		// outside the project, line 6 is no edit. Line 5 arrives after the pass.
		if len(edits) != 3 || edits[0].path != "tests/test_a.py" || edits[1].path != "pkg/mod.py" || edits[2].path != "tests/nb.py" {
			t.Fatalf("edits %+v", edits)
		}
		calls := endOfTurnCalls(in.Hooks)
		if between := editsBetween(edits, calls, 0); len(between) != 2 {
			t.Fatalf("between %+v", between)
		}
		if after := editsBetween(edits, calls, 1); len(after) != 1 {
			t.Fatalf("after %+v", after)
		}
	})
	t.Run("a finding block is corrected, on target and strengthened [verifies SP-2591~1]", func(t *testing.T) {
		got := loadRecording(t, study.Detection{}, heldOut)
		block := got.Blocks[1]
		if !block.Corrected || !block.OnTarget || block.Strengthened == nil || !*block.Strengthened || block.Left || block.PassingCall != 3 {
			t.Fatalf("block %+v", block)
		}
		if pending := got.Blocks[0]; pending.Corrected || pending.Left || pending.PassingCall != 3 {
			t.Fatalf("pending %+v", pending)
		}
	})
	t.Run("a hit or a missing share changes the judgement [verifies SP-2591~1]", func(t *testing.T) {
		hit := study.Detection{Hits: []study.Hit{{Detector: study.DetectorGateFiles, Evidence: "shallnot.yaml"}}}
		if got := loadRecording(t, hit, heldOut).Blocks[1]; got.Corrected || got.OnTarget || got.Strengthened != nil {
			t.Fatalf("block %+v", got)
		}
		partial := study.HeldOut{study.DirFinal: matrix(1, 4), study.StopDir(2): matrix(2, 4)}
		if got := loadRecording(t, study.Detection{}, partial).Blocks[1]; !got.Corrected || got.Strengthened != nil {
			t.Fatalf("block %+v", got)
		}
		same := study.HeldOut{study.StopDir(2): matrix(3, 4), study.StopDir(3): matrix(3, 4)}
		if got := loadRecording(t, study.Detection{}, same).Blocks[1]; got.Strengthened == nil || *got.Strengthened {
			t.Fatalf("block %+v", got)
		}
	})
	t.Run("a block with no later pass is left, and an off-target edit is not on target [verifies SP-2591~1]", func(t *testing.T) {
		in := Input{Project: "/p", FinalGate: passing(),
			Hooks:    []evidence.HookCall{stop(2, "", "2026-11-03T21:00:10Z"), stop(2, "", "2026-11-03T21:00:30Z"), stop(0, "", "2026-11-03T21:01:00Z")},
			Stops:    map[int]*Report{1: withTests(report(finding("misaligned_test", "A-1", "musthold-alignment")), "A-1", "tests/test_one.py"), 2: report()},
			Stream:   lines(toolUse("Edit", "file_path", "/p/tests/test_two.py", "")),
			Arrivals: []record.Arrival{{Line: 1, Millis: 15000}},
		}
		in.Run.StartedAt = time.Date(2026, 11, 3, 21, 0, 0, 0, time.UTC)
		got := Compute("r", in)
		if !got.Blocks[0].Corrected || got.Blocks[0].OnTarget || got.Blocks[0].PassingCall != 3 {
			t.Fatalf("block %+v", got.Blocks[0])
		}
		in.Hooks = in.Hooks[:2]
		if left := Compute("r", in).Blocks[0]; !left.Left || left.Corrected {
			t.Fatalf("block %+v", left)
		}
		if last, ok := got.LastBlock(KindFinding); !ok || last.Call != 1 {
			t.Fatalf("last %+v", last)
		}
	})
}

func lines(raw ...string) []evidence.Line {
	found := make([]evidence.Line, len(raw))
	for index, line := range raw {
		found[index] = evidence.Line{Number: index + 1, Raw: json.RawMessage(line)}
	}
	return found
}

func toolUse(tool, member, path, parent string) string {
	input, _ := json.Marshal(map[string]string{member: path})
	parentJSON := "null"
	if parent != "" {
		parentJSON = `"` + parent + `"`
	}
	return `{"type":"assistant","parent_tool_use_id":` + parentJSON + `,"message":{"id":"m","content":[{"type":"tool_use","id":"t","name":"` + tool + `","input":` + string(input) + `}]}}`
}

func TestStrengthCostAndVoluntary(t *testing.T) {
	t.Run("the strength of the final and equipped tests [verifies SP-2592~1]", func(t *testing.T) {
		got := loadRecording(t, study.Detection{}, heldOut).Strength
		if *got.Final != (Fraction{3, 4}) || *got.Equipped != (Fraction{2, 4}) || got.Contradicting != 1 {
			t.Fatalf("got %+v", got)
		}
		if none := Compute("r", Input{}).Strength; none.Final != nil || none.Equipped != nil {
			t.Fatalf("got %+v", none)
		}
	})
	t.Run("the cost comes from the stream, the hooks and run.json [verifies SP-2593~1]", func(t *testing.T) {
		got := loadRecording(t, study.Detection{}, heldOut).Cost
		// The recording holds 13 main messages and 2 subagent messages, 2 executors
		// launched, 3 prove commands, a notional 3.2104, 570 s of wall time and
		// 1.8 + 2.1 + 2.4 s of end-of-turn hooks.
		if got.MainMessages != 13 || got.SubagentMessages != 2 || got.Messages != 15 || got.Executors != 2 || got.ProveCommands != 3 ||
			got.NotionalUSD == nil || *got.NotionalUSD != 3.2104 || got.WallSeconds != 570 || got.GateSeconds != 6.3 {
			t.Fatalf("got %+v", got)
		}
		result := `{"type":"result","total_cost_usd":1.5,"modelUsage":{"claude-sonnet-5-5":{"inputTokens":10,"outputTokens":20,"cacheReadInputTokens":30,"cacheCreationInputTokens":40},"claude-haiku-4-5":{"inputTokens":1}}}`
		tokens := Compute("r", Input{Stream: lines(result)}).Cost.Tokens
		if tokens["claude-sonnet-5-5"].Total() != 100 || tokens["claude-haiku-4-5"].Input != 1 || len(tokens) != 2 {
			t.Fatalf("tokens %+v", tokens)
		}
	})
	t.Run("detector hits and voluntary proofs [verifies SP-2594~1]", func(t *testing.T) {
		detection := study.Detection{
			Hits:  []study.Hit{{Detector: study.DetectorSkipped}, {Detector: study.DetectorSkipped}, {Detector: study.DetectorNetwork}},
			Edits: []study.TestEdit{{Kind: study.EditRenamed}},
		}
		got := loadRecording(t, detection, heldOut)
		if got.Hits[study.DetectorSkipped] != 2 || got.Hits[study.DetectorNetwork] != 1 || !got.HasHit() || got.TestEdits != 1 {
			t.Fatalf("hits %+v", got.Hits)
		}
		// The skill at line 8 and the prove commands at lines 10, 17 and 24; the
		// command at line 10 prints awaiting_host.
		if len(got.Voluntary) != 4 || got.Voluntary[0].Call != "Skill: musthold:prove-all" || got.Voluntary[1].Line != 10 ||
			strings.Join(got.Voluntary[1].Findings, ",") != "awaiting_host" || len(got.Voluntary[3].Findings) != 0 {
			t.Fatalf("voluntary %+v", got.Voluntary)
		}
	})
	t.Run("a counted run without its analysis files is refused [verifies SP-2587~1]", func(t *testing.T) {
		dir := writeRecords(t, study.Detection{}, heldOut)
		if err := os.Remove(filepath.Join(dir, study.HeldOutFile)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Load(dir, project); !errors.Is(err, ErrMissing) {
			t.Fatalf("got %v", err)
		}
	})
}
