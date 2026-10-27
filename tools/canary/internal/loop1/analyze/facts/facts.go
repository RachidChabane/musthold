// Package facts computes the measures of PREREG §9 for one counted run of the
// loop-1 study, from its records and the files the oracle and the detectors
// added: acceptance, defect and escape; the end-of-turn calls with each block's
// kind; the correction of each block and its companions; the strength of the
// tests; the detector hits; the cost; and the voluntary proofs. Compute is pure;
// Load reads a records directory.
package facts

import (
	"encoding/json"
	"math/big"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/transcript"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Report is the part of Shallnot's JSON report the facts read: the gate's
// verdict, its findings, and the tests bound to each requirement.
type Report struct {
	record.GateReport
	Requirements []Requirement `json:"requirements"`
}

// Requirement is one requirement of a report with the tests that cite it.
type Requirement struct {
	ID    string `json:"id"`
	Tests []struct {
		Name   string `json:"name"`
		Source struct {
			File string `json:"file"`
		} `json:"source"`
	} `json:"tests"`
}

// DecodeReport reads a report; an empty file is no report.
func DecodeReport(data []byte) (*Report, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

// passVerdict is the verdict of a gate that passes.
const passVerdict = "pass"

// Passes reports whether the report is a passing gate.
func (r *Report) Passes() bool { return r != nil && r.ExitCode == 0 && r.Verdict == passVerdict }

// testFiles lists the files of the tests that cite the criterion.
func (r *Report) testFiles(criterion string) []string {
	if r == nil {
		return nil
	}
	var files []string
	for _, requirement := range r.Requirements {
		if requirement.ID != criterion {
			continue
		}
		for _, test := range requirement.Tests {
			if test.Source.File != "" {
				files = append(files, path.Clean(test.Source.File))
			}
		}
	}
	return files
}

// Input is a counted run's records, decoded.
type Input struct {
	Run      study.Run
	Stream   []evidence.Line
	Arrivals []record.Arrival
	Hooks    []evidence.HookCall
	// Stops maps the number of each end-of-turn call to its report, nil when the
	// shim wrote none.
	Stops map[int]*Report
	// FinalGate is gate-after.json, the acceptance gate.
	FinalGate *Report
	Oracle    study.Oracle
	HeldOut   study.HeldOut
	Detection study.Detection
	// Project is the absolute path of the project the session worked in.
	Project string
}

// Decision is what one end-of-turn call decided.
type Decision string

// The decisions of an end-of-turn call.
const (
	DecisionBlock  Decision = "block"
	DecisionGiveUp Decision = "give-up"
	DecisionPass   Decision = "pass"
	// DecisionOther is an exit 1 without the give-up message, or any other exit.
	DecisionOther Decision = "other"
)

// BlockKind is the kind of a block (PREREG §9.5).
type BlockKind string

// The kinds of block, in the order a block takes them.
const (
	KindFinding      BlockKind = "finding"
	KindPending      BlockKind = "pending"
	KindTraceability BlockKind = "traceability"
)

// BlockKinds lists the kinds in order.
func BlockKinds() []BlockKind { return []BlockKind{KindFinding, KindPending, KindTraceability} }

// Exit codes and messages of Shallnot's end-of-turn hook.
const (
	exitPass   = 0
	exitGiveUp = 1
	exitBlock  = 2
	// GiveUpMessage begins the message of a hook that lets the turn end with the
	// gate still blocked.
	GiveUpMessage   = "shallnot: the gate is still blocked after"
	shallnotModule  = "shallnot"
	hookSubcommand  = "hook"
	stopHookCommand = "claude-stop"
	// staleProof is the proof finding of a missing or stale proof.
	staleProof = "stale_proof"
)

// Finding is one blocking finding of a block, by code and criterion.
type Finding struct {
	Code      string `json:"code"`
	Criterion string `json:"criterion"`
	Module    string `json:"module,omitempty"`
}

// Call is one end-of-turn call of Shallnot.
type Call struct {
	// N counts the calls from 1; HooksLine is the call's line in hooks.jsonl.
	N         int      `json:"n"`
	HooksLine int      `json:"hooksLine"`
	ExitCode  int      `json:"exitCode"`
	Decision  Decision `json:"decision"`
	// StartedAt is when the hook started; DurationMillis how long it ran.
	StartedAt      time.Time `json:"startedAt"`
	DurationMillis int64     `json:"durationMs"`
}

// Block is one block with its kind and how the session answered it.
type Block struct {
	Call int       `json:"call"`
	Kind BlockKind `json:"kind"`
	// ReportMissing says that stops/<n>/report.json was absent, so the block
	// carries no finding and is a traceability block.
	ReportMissing bool `json:"reportMissing,omitempty"`
	// Findings are the blocking findings of a finding block's proof verdicts, or
	// every blocking finding of another kind of block.
	Findings []Finding `json:"findings"`
	// Edits counts the source edits between this call and the next.
	Edits int `json:"edits"`
	// PassingCall is the first later call that passes, or 0.
	PassingCall int  `json:"passingCall"`
	Corrected   bool `json:"corrected"`
	OnTarget    bool `json:"onTarget"`
	// Strengthened is null when a kill share it compares is missing.
	Strengthened *bool `json:"strengthened"`
	Left         bool  `json:"left"`
}

// Fraction is k of n.
type Fraction struct {
	K int `json:"k"`
	N int `json:"n"`
}

// Rat is the fraction as an exact rational; 0 of 0 is 0.
func (f Fraction) Rat() *big.Rat {
	if f.N == 0 {
		return new(big.Rat)
	}
	return big.NewRat(int64(f.K), int64(f.N))
}

// Strength is the strength of the run's tests (PREREG §9.7); a share is null
// when heldout.json has no matrix for it.
type Strength struct {
	Final         *Fraction `json:"final"`
	Equipped      *Fraction `json:"equipped"`
	Contradicting int       `json:"contradicting"`
}

// Tokens is one model's token counts from the result event.
type Tokens struct {
	Input         int64 `json:"input"`
	Output        int64 `json:"output"`
	CacheRead     int64 `json:"cacheRead"`
	CacheCreation int64 `json:"cacheCreation"`
}

// Total is every token of the model.
func (t Tokens) Total() int64 { return t.Input + t.Output + t.CacheRead + t.CacheCreation }

// Cost is the cost of the run (PREREG §9.9).
type Cost struct {
	MainMessages     int `json:"mainMessages"`
	SubagentMessages int `json:"subagentMessages"`
	Messages         int `json:"messages"`
	Executors        int `json:"executors"`
	ProveCommands    int `json:"proveCommands"`
	// NotionalUSD is the CLI's total_cost_usd, notional under a subscription
	// token; null when the stream has no result event.
	NotionalUSD *float64          `json:"notionalUsd"`
	Tokens      map[string]Tokens `json:"tokens"`
	WallSeconds float64           `json:"wallSeconds"`
	GateSeconds float64           `json:"gateSeconds"`
}

// Voluntary is one proof the session ran of its own accord: a module's prove
// command or the prove-all skill, with the finding codes its output prints.
type Voluntary struct {
	Line     int      `json:"line"`
	Call     string   `json:"call"`
	Findings []string `json:"findings"`
}

// Facts is facts.json: every measure of PREREG §9 for one counted run.
type Facts struct {
	Run        string        `json:"run"`
	Task       string        `json:"task"`
	Arm        string        `json:"arm"`
	Repetition int           `json:"repetition"`
	Attempt    int           `json:"attempt"`
	Outcome    study.Outcome `json:"outcome"`
	Capped     bool          `json:"capped"`
	Accepted   bool          `json:"accepted"`
	Defective  bool          `json:"defective"`
	Escaped    bool          `json:"escaped"`
	Violated   []string      `json:"violated"`
	Flaky      []string      `json:"flaky"`
	Calls      []Call        `json:"calls"`
	Blocks     []Block       `json:"blocks"`
	// BlockCounts counts the blocks of each kind; GiveUps the give-ups.
	BlockCounts map[BlockKind]int `json:"blockCounts"`
	GiveUps     int               `json:"giveUps"`
	// FindingCodes counts the findings of the finding blocks by code.
	FindingCodes map[string]int `json:"findingCodes"`
	Strength     Strength       `json:"strength"`
	// Hits counts the detector hits by detector; TestEdits the reported edits to
	// pre-existing tests.
	Hits      map[study.Detector]int `json:"hits"`
	TestEdits int                    `json:"testEdits"`
	Cost      Cost                   `json:"cost"`
	Voluntary []Voluntary            `json:"voluntary"`
}

// HasHit reports whether any detector has a hit on the run.
func (f Facts) HasHit() bool {
	for _, count := range f.Hits {
		if count > 0 {
			return true
		}
	}
	return false
}

// LastBlock returns the last block of the kind.
func (f Facts) LastBlock(kind BlockKind) (Block, bool) {
	for index := len(f.Blocks) - 1; index >= 0; index-- {
		if f.Blocks[index].Kind == kind {
			return f.Blocks[index], true
		}
	}
	return Block{}, false
}

// Compute computes the facts of a counted run named name.
func Compute(name string, in Input) Facts {
	facts := Facts{
		Run: name, Task: in.Run.Task, Arm: in.Run.Arm, Repetition: in.Run.Repetition, Attempt: in.Run.Attempt,
		Outcome: in.Run.Outcome, Capped: in.Run.Outcome.Capped(),
		Accepted: in.FinalGate.Passes(), Defective: in.Oracle.Defective,
		Violated: orEmpty(in.Oracle.Violated), Flaky: orEmpty(in.Oracle.Flaky),
		BlockCounts: map[BlockKind]int{}, FindingCodes: map[string]int{},
		Hits: map[study.Detector]int{}, TestEdits: len(in.Detection.Edits),
	}
	facts.Escaped = facts.Accepted && facts.Defective
	for _, hit := range in.Detection.Hits {
		facts.Hits[hit.Detector]++
	}
	facts.Calls = endOfTurnCalls(in.Hooks)
	edits := placedEdits(in)
	facts.Blocks = blocks(in, facts.Calls, edits, facts.Accepted && !facts.HasHit())
	for _, call := range facts.Calls {
		if call.Decision == DecisionGiveUp {
			facts.GiveUps++
		}
	}
	for _, block := range facts.Blocks {
		facts.BlockCounts[block.Kind]++
		if block.Kind == KindFinding {
			for _, finding := range block.Findings {
				facts.FindingCodes[finding.Code]++
			}
		}
	}
	facts.Strength = strength(in.HeldOut)
	facts.Cost = cost(in)
	facts.Voluntary = voluntary(in.Stream)
	return facts
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// endOfTurnCalls numbers Shallnot's end-of-turn hook records from 1.
func endOfTurnCalls(hooks []evidence.HookCall) []Call {
	calls := []Call{}
	for _, hook := range hooks {
		if hook.Module != shallnotModule || len(hook.Args) < 2 || hook.Args[0] != hookSubcommand || hook.Args[1] != stopHookCommand {
			continue
		}
		started, _ := time.Parse(time.RFC3339Nano, hook.StartedAt)
		calls = append(calls, Call{
			N: len(calls) + 1, HooksLine: hook.Line, ExitCode: hook.ExitCode, Decision: decide(hook),
			StartedAt: started, DurationMillis: hook.DurationMillis,
		})
	}
	return calls
}

func decide(hook evidence.HookCall) Decision {
	switch {
	case hook.ExitCode == exitBlock:
		return DecisionBlock
	case hook.ExitCode == exitPass:
		return DecisionPass
	case hook.ExitCode == exitGiveUp && strings.Contains(hook.Stderr, GiveUpMessage):
		return DecisionGiveUp
	default:
		return DecisionOther
	}
}

// isPending reports whether a proof finding code is stale_proof or a pending
// reason.
func isPending(code string) bool {
	return code == staleProof || slices.Contains(format.PendingReasons(), format.PendingReason(code))
}

// classify gives a block its kind and findings from its report. A proof finding
// is one that names its module.
func classify(report *Report) (BlockKind, []Finding) {
	var verdicts, pending, other []Finding
	if report != nil {
		for _, finding := range report.Findings {
			if !finding.Blocking {
				continue
			}
			found := Finding{Code: finding.Category, Criterion: finding.RequirementID, Module: finding.Module}
			switch {
			case finding.Module != "" && !isPending(finding.Category):
				verdicts = append(verdicts, found)
			case finding.Module != "" || isPending(finding.Category):
				pending = append(pending, found)
			default:
				other = append(other, found)
			}
		}
	}
	switch {
	case len(verdicts) > 0:
		return KindFinding, verdicts
	case len(pending) > 0:
		return KindPending, append(pending, other...)
	default:
		return KindTraceability, orFindings(other)
	}
}

func orFindings(found []Finding) []Finding {
	if found == nil {
		return []Finding{}
	}
	return found
}

// edit is one edit tool call of the stream, placed in time.
type edit struct {
	at      time.Time
	path    string
	text    string
	placed  bool
	isTests bool
}

var editTools = []string{"Edit", "Write", "MultiEdit", "NotebookEdit"}

// testsDir is the project's test directory, and pythonSuffix the files of its
// source.
const (
	testsDir     = "tests"
	pythonSuffix = ".py"
)

// placedEdits reads every edit of the stream, the subagents' included, that
// touches the project's tests or its Python source, at the run's start plus its
// line's arrival time.
func placedEdits(in Input) []edit {
	arrivals := map[int]int64{}
	for _, arrival := range in.Arrivals {
		arrivals[arrival.Line] = arrival.Millis
	}
	var edits []edit
	for _, call := range evidence.DecodeClaude(in.Stream).ToolUses("") {
		if !slices.Contains(editTools, call.Name) {
			continue
		}
		var input map[string]any
		_ = json.Unmarshal(call.Input, &input)
		rel, inside := projectPath(in.Project, firstString(input, "file_path", "notebook_path"))
		isTests := strings.HasPrefix(rel, testsDir+"/")
		if !inside || !(isTests || strings.HasSuffix(rel, pythonSuffix)) {
			continue
		}
		found := edit{path: rel, text: string(call.Input), isTests: isTests}
		if millis, ok := arrivalAt(arrivals, call.Line); ok && !in.Run.StartedAt.IsZero() {
			found.at, found.placed = in.Run.StartedAt.Add(time.Duration(millis)*time.Millisecond), true
		}
		edits = append(edits, found)
	}
	return edits
}

// arrivalAt is the arrival time of the line, or of the last earlier line that
// has one.
func arrivalAt(arrivals map[int]int64, line int) (int64, bool) {
	for at := line; at > 0; at-- {
		if millis, found := arrivals[at]; found {
			return millis, true
		}
	}
	return 0, false
}

func firstString(input map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := input[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

// projectPath gives a tool's path relative to the project, and whether it lies
// inside it: a path under the project's own path, or a relative path that does
// not leave it.
func projectPath(project, target string) (string, bool) {
	if target == "" {
		return "", false
	}
	if rel, found := strings.CutPrefix(filepath.ToSlash(target), filepath.ToSlash(project)+"/"); found {
		return path.Clean(rel), true
	}
	if filepath.IsAbs(target) {
		return "", false
	}
	clean := path.Clean(filepath.ToSlash(target))
	return clean, clean != ".." && !strings.HasPrefix(clean, "../")
}

// editsBetween returns the placed edits after the start of call n and before the
// start of the next call, or after call n when it is the last.
func editsBetween(edits []edit, calls []Call, index int) []edit {
	var found []edit
	from := calls[index].StartedAt
	for _, e := range edits {
		if !e.placed || !e.at.After(from) {
			continue
		}
		if index+1 < len(calls) && !e.at.Before(calls[index+1].StartedAt) {
			continue
		}
		found = append(found, e)
	}
	return found
}

// blocks judges every block. clean says the change is accepted and no detector
// has a hit, the two conditions of a correction that do not depend on the block.
func blocks(in Input, calls []Call, edits []edit, clean bool) []Block {
	judged := []Block{}
	for index, call := range calls {
		if call.Decision != DecisionBlock {
			continue
		}
		report, reported := in.Stops[call.N]
		kind, findings := classify(report)
		block := Block{Call: call.N, Kind: kind, ReportMissing: !reported || report == nil, Findings: findings}
		between := editsBetween(edits, calls, index)
		block.Edits = len(between)
		for _, later := range calls[index+1:] {
			if later.Decision == DecisionPass {
				block.PassingCall = later.N
				break
			}
		}
		block.Left = block.PassingCall == 0
		block.Corrected = block.Edits > 0 && !block.Left && clean
		if block.Corrected {
			block.OnTarget = onTarget(between, findings, report, in.Stops[block.PassingCall])
			block.Strengthened = strengthened(in.HeldOut, call.N, block.PassingCall)
		}
		judged = append(judged, block)
	}
	return judged
}

// citationMark is the revision separator of a citation, as in BOX-2~1.
const citationMark = "~"

// onTarget reports whether an edit touches a test file that cites a criterion of
// the findings, at the blocked call or at the passing call, or writes a citation
// of one.
func onTarget(edits []edit, findings []Finding, blocked, passing *Report) bool {
	for _, finding := range findings {
		files := append(blocked.testFiles(finding.Criterion), passing.testFiles(finding.Criterion)...)
		for _, e := range edits {
			if !e.isTests {
				continue
			}
			if slices.Contains(files, e.path) || (finding.Criterion != "" && strings.Contains(e.text, finding.Criterion+citationMark)) {
				return true
			}
		}
	}
	return false
}

// strengthened compares the kill share of the tests at the passing call with
// that at the blocked call; it is nil when either is missing.
func strengthened(heldOut study.HeldOut, blocked, passing int) *bool {
	before, foundBefore := heldOut[study.StopDir(blocked)]
	after, foundAfter := heldOut[study.StopDir(passing)]
	if !foundBefore || !foundAfter {
		return nil
	}
	b, a := share(before), share(after)
	above := a.Rat().Cmp(b.Rat()) > 0
	return &above
}

func share(matrix study.KillMatrix) Fraction {
	killed, total := matrix.Share()
	return Fraction{K: killed, N: total}
}

func strength(heldOut study.HeldOut) Strength {
	var found Strength
	if final, ok := heldOut[study.DirFinal]; ok {
		s := share(final)
		found.Final, found.Contradicting = &s, len(final.Contradicting)
	}
	if equipped, ok := heldOut[study.DirEquipped]; ok {
		s := share(equipped)
		found.Equipped = &s
	}
	return found
}

const millisPerSecond = 1000.0

func cost(in Input) Cost {
	stats := transcript.Build(transcript.Input{Meta: in.Run.Meta, Stream: in.Stream, Arrivals: in.Arrivals, Hooks: in.Hooks}).Stats
	found := Cost{
		MainMessages: stats.MainMessages, SubagentMessages: stats.SubagentMessages,
		Messages:  stats.MainMessages + stats.SubagentMessages,
		Executors: stats.Executors, ProveCommands: stats.ProveRuns,
		Tokens:      modelTokens(in.Stream),
		WallSeconds: float64(stats.SessionMillis) / millisPerSecond,
		GateSeconds: float64(stats.GateMillis) / millisPerSecond,
	}
	if stats.Result != nil {
		usd := stats.Result.TotalCostUSD
		found.NotionalUSD = &usd
	}
	return found
}

// modelTokens reads the tokens per model of the stream's result event.
func modelTokens(lines []evidence.Line) map[string]Tokens {
	tokens := map[string]Tokens{}
	for _, line := range lines {
		var raw struct {
			Type       string `json:"type"`
			ModelUsage map[string]struct {
				Input         int64 `json:"inputTokens"`
				Output        int64 `json:"outputTokens"`
				CacheRead     int64 `json:"cacheReadInputTokens"`
				CacheCreation int64 `json:"cacheCreationInputTokens"`
			} `json:"modelUsage"`
		}
		if json.Unmarshal(line.Raw, &raw) != nil || raw.Type != evidence.ClaudeTypeResult {
			continue
		}
		for model, usage := range raw.ModelUsage {
			tokens[model] = Tokens{Input: usage.Input, Output: usage.Output, CacheRead: usage.CacheRead, CacheCreation: usage.CacheCreation}
		}
		break
	}
	return tokens
}
