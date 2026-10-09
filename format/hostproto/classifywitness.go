package hostproto

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/format"
)

// Limits of the raw API log the witness reads.
const (
	// MaxWitnessIndexBytes is the largest index the check reads.
	MaxWitnessIndexBytes = 32 << 20
	// MaxWitnessBodyBytes is the largest line or body file the check reads.
	MaxWitnessBodyBytes = 4 << 20
	// WitnessIndexFile is the index the host writes next to the bodies.
	WitnessIndexFile = "index.jsonl"

	userRole = "user"
)

// WitnessVerdict is what the check of the raw API log came to.
type WitnessVerdict string

// Verdicts of the check.
const (
	// WitnessRaised: every dictated call is in the log, blind, with the labels
	// that were submitted. The votes keep the isolation `restricted`.
	WitnessRaised WitnessVerdict = "raised"
	// WitnessExposed: a logged call shares the dictated text but carried history,
	// tools or another system prompt. The classification was not blind.
	WitnessExposed WitnessVerdict = "exposed"
	// WitnessLabelsContradicted: the log holds the dictated calls with other labels
	// than the submitted ones.
	WitnessLabelsContradicted WitnessVerdict = "labels_contradicted"
	// WitnessNone: the log does not settle the matter; nothing is raised and
	// nothing is marked.
	WitnessNone WitnessVerdict = "none"
)

// Reasons the check raises nothing and marks nothing.
const (
	WitnessReasonVersion    = "cli_version_not_pinned"
	WitnessReasonLog        = "log_unreadable_or_oversize"
	WitnessReasonMissing    = "dictated_call_not_logged"
	WitnessReasonExtra      = "extra_logged_call"
	WitnessReasonConsumed   = "log_entry_already_consumed"
	WitnessReasonNoWindow   = "no_time_window"
	WitnessReasonNoLabelled = "no_calls"
)

// WitnessLog is the raw API log of the user's host: its index, and the bodies the
// index names.
type WitnessLog interface {
	// Index returns the lines of the index.
	Index() ([]byte, error)
	// File returns one request or response body by the name the index gives.
	File(name string) ([]byte, error)
}

// FileReader reads one file of a raw API log, refusing a file larger than the
// limit. The adapter that opens the directory implements it.
type FileReader interface {
	ReadFile(name string, limit int) ([]byte, error)
}

// FileLog is a raw API log held in files: the index, and the bodies the index
// names, which must lie inside the log's directory.
type FileLog struct {
	Files FileReader
	// Root is the directory of the log, so that a file named by its absolute path
	// can be found.
	Root string
}

// Index implements WitnessLog.
func (l FileLog) Index() ([]byte, error) {
	return l.Files.ReadFile(WitnessIndexFile, MaxWitnessIndexBytes)
}

// File implements WitnessLog.
func (l FileLog) File(name string) ([]byte, error) {
	relative := name
	if filepath.IsAbs(name) && l.Root != "" {
		var err error
		if relative, err = filepath.Rel(l.Root, name); err != nil {
			return nil, err
		}
	}
	relative = path.Clean(filepath.ToSlash(relative))
	if relative == ".." || strings.HasPrefix(relative, "../") || strings.HasPrefix(relative, "/") || relative == "." {
		return nil, fmt.Errorf("the file %q is outside the log", name)
	}
	return l.Files.ReadFile(relative, MaxWitnessBodyBytes)
}

// WitnessInput is what the check needs of one classification.
type WitnessInput struct {
	Record format.ClassifyRecord
	// Texts maps each question to the text of each of its calls, as the plan
	// dictated it.
	Texts map[string][]string
	// From and To bound the part of the log that may hold the calls: the ticket was
	// emitted at From and the classification was imported at To.
	From, To time.Time
	// Consumed holds the request ids of log entries an earlier witness used.
	Consumed map[string]bool
	// AnyQuerySource reads the entries of every query source instead of the
	// classifier's, and reports the sources the matching entries carry. It serves
	// the observation of a classification that answers nothing.
	AnyQuerySource bool
}

// WitnessResult is the outcome of the check.
type WitnessResult struct {
	Verdict WitnessVerdict
	Reason  string
	// RequestIDs are the log entries the dictated calls matched.
	RequestIDs []string
	// Model is the model the log shows answered the calls, the commonest first.
	Model  string
	Models []string
	// QuerySources are the query sources of the matching entries, sorted.
	QuerySources []string
	// LoggedCalls is the number of index lines the check read.
	LoggedCalls int
}

// indexLine is one line of the index of the raw API log.
type indexLine struct {
	Timestamp    json.RawMessage `json:"timestamp"`
	SessionID    string          `json:"session_id"`
	QuerySource  string          `json:"query_source"`
	Model        string          `json:"model"`
	RequestID    string          `json:"request_id"`
	RequestFile  string          `json:"request_file"`
	ResponseFile string          `json:"response_file"`
}

// loggedCall is an index line with the bodies it names.
type loggedCall struct {
	line     indexLine
	at       time.Time
	request  classifierRequest
	response string
}

// classifierRequest is the part of a request body the check reads.
type classifierRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	System    json.RawMessage   `json:"system"`
	Messages  []requestMessage  `json:"messages"`
	Tools     []json.RawMessage `json:"tools"`
}

type requestMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type responseBody struct {
	Content json.RawMessage `json:"content"`
}

// dictatedCall is one call the plan dictated: its system prompt, its user prompt
// and the label submitted for it, none for a missing vote.
type dictatedCall struct {
	system string
	user   string
	label  *string
	labels []string
}

// CheckClassifyWitness decides whether the raw API log holds the classification:
// every dictated call exactly once, with the dictated system prompt and text, the
// token limit of the classifier, no tools and no history, inside the time window,
// and with responses that read, as a multiset, as the labels that were submitted.
func CheckClassifyWitness(log WitnessLog, in WitnessInput) WitnessResult {
	none := func(reason string) WitnessResult { return WitnessResult{Verdict: WitnessNone, Reason: reason} }
	switch {
	case !slices.Contains(ClassifyPinnedCLIVersions(), in.Record.CLIVersion):
		return none(WitnessReasonVersion)
	case in.From.IsZero() || in.To.IsZero():
		return none(WitnessReasonNoWindow)
	case len(in.Record.Calls) == 0:
		return none(WitnessReasonNoLabelled)
	}
	dictated, err := dictatedCalls(in)
	if err != nil {
		return none(WitnessReasonNoLabelled)
	}
	logged, err := readLoggedCalls(log, in.AnyQuerySource)
	if err != nil {
		return none(WitnessReasonLog)
	}
	windowed := slices.DeleteFunc(slices.Clone(logged), func(call loggedCall) bool {
		return call.at.IsZero() || call.at.Before(in.From) || call.at.After(in.To)
	})
	result := judgeWitness(dictated, windowed, in)
	result.LoggedCalls = len(logged)
	return result
}

func dictatedCalls(in WitnessInput) ([]dictatedCall, error) {
	calls := make([]dictatedCall, 0, len(in.Record.Calls))
	for _, call := range in.Record.Calls {
		texts := in.Texts[call.Question]
		if call.Index < 0 || call.Index >= len(texts) {
			return nil, fmt.Errorf("no text for call %d of %q", call.Index, call.Question)
		}
		calls = append(calls, dictatedCall{
			system: ClassifierSystemPrompt(call.Order), user: ClassifierUserPrompt(texts[call.Index]), label: call.Label, labels: call.Order,
		})
	}
	return calls, nil
}

func readLoggedCalls(log WitnessLog, anySource bool) ([]loggedCall, error) {
	index, err := log.Index()
	if err != nil {
		return nil, err
	}
	var calls []loggedCall
	scanner := bufio.NewScanner(bytes.NewReader(index))
	scanner.Buffer(make([]byte, 0, 64<<10), MaxWitnessBodyBytes)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var line indexLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil || (!anySource && line.QuerySource != ClassifyQuerySource) {
			continue
		}
		call := loggedCall{line: line, at: parseLogTime(line.Timestamp)}
		if body, err := log.File(line.RequestFile); err == nil {
			_ = json.Unmarshal(body, &call.request)
		}
		if body, err := log.File(line.ResponseFile); err == nil {
			call.response = responseText(body)
		}
		calls = append(calls, call)
	}
	return calls, scanner.Err()
}

// parseLogTime reads a timestamp written as an RFC 3339 string or as a number of
// milliseconds since the epoch; an unreadable one is the zero time.
func parseLogTime(raw json.RawMessage) time.Time {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return parsed
		}
		return time.Time{}
	}
	var millis float64
	if json.Unmarshal(raw, &millis) == nil {
		return time.UnixMilli(int64(millis))
	}
	return time.Time{}
}

// responseText joins the text blocks of a response body, or reads it as a bare
// string.
func responseText(body []byte) string {
	var parsed responseBody
	if json.Unmarshal(body, &parsed) != nil {
		return ""
	}
	return textOf(parsed.Content)
}

// textOf reads a content member: a string, or blocks whose text members are joined.
func textOf(content json.RawMessage) string {
	var plain string
	if json.Unmarshal(content, &plain) == nil {
		return plain
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return ""
	}
	var joined strings.Builder
	for _, block := range blocks {
		if block.Type == "text" || block.Type == "" {
			joined.WriteString(block.Text)
		}
	}
	return joined.String()
}

// blind reports whether a logged request is the dictated one: the classifier's
// token limit, no tools, the system prompt and one user message that are dictated.
func (c loggedCall) blind(call dictatedCall) bool {
	return c.request.MaxTokens == ClassifyMaxTokens && len(c.request.Tools) == 0 &&
		textOf(c.request.System) == call.system && len(c.request.Messages) == 1 &&
		c.request.Messages[0].Role == userRole && textOf(c.request.Messages[0].Content) == call.user
}

// leaks reports whether a logged request carries the dictated text with history,
// tools or another system prompt: the call was not the blind classifier request
// that was dictated.
func (c loggedCall) leaks(call dictatedCall) bool {
	carries := slices.ContainsFunc(c.request.Messages, func(message requestMessage) bool {
		return message.Role == userRole && strings.Contains(textOf(message.Content), call.user)
	})
	return carries && (len(c.request.Messages) > 1 || len(c.request.Tools) > 0 || textOf(c.request.System) != call.system)
}

// callBody is what a dictated call sends the classifier: its system prompt and its
// user message.
type callBody struct{ system, user string }

func judgeWitness(dictated []dictatedCall, logged []loggedCall, in WitnessInput) WitnessResult {
	for _, call := range logged {
		if slices.ContainsFunc(dictated, call.leaks) {
			return WitnessResult{Verdict: WitnessExposed}
		}
	}
	wanted := map[callBody]int{}
	for _, call := range dictated {
		wanted[callBody{call.system, call.user}]++
	}
	matched := map[callBody][]loggedCall{}
	for _, call := range logged {
		for key := range wanted {
			if call.blind(dictatedCall{system: key.system, user: key.user}) {
				matched[key] = append(matched[key], call)
			}
		}
	}
	var requestIDs []string
	for key, count := range wanted {
		switch found := len(matched[key]); {
		case found < count:
			return WitnessResult{Verdict: WitnessNone, Reason: WitnessReasonMissing}
		case found > count:
			return WitnessResult{Verdict: WitnessNone, Reason: WitnessReasonExtra}
		}
		for _, call := range matched[key] {
			if in.Consumed[call.line.RequestID] || call.line.RequestID == "" {
				return WitnessResult{Verdict: WitnessNone, Reason: WitnessReasonConsumed}
			}
			requestIDs = append(requestIDs, call.line.RequestID)
		}
	}
	slices.Sort(requestIDs)
	if len(slices.Compact(slices.Clone(requestIDs))) != len(requestIDs) {
		return WitnessResult{Verdict: WitnessNone, Reason: WitnessReasonConsumed}
	}
	return compareLabels(dictated, matched, requestIDs)
}

// compareLabels checks, per dictated body, that the responses read as the labels
// that were submitted for it, as multisets.
func compareLabels(dictated []dictatedCall, matched map[callBody][]loggedCall, requestIDs []string) WitnessResult {
	models, sources := map[string]int{}, map[string]bool{}
	for key, calls := range matched {
		var submitted, answered []string
		for _, call := range dictated {
			if call.system == key.system && call.user == key.user {
				submitted = append(submitted, labelOrNone(call.label))
			}
		}
		for _, call := range calls {
			models[call.line.Model]++
			sources[call.line.QuerySource] = true
			label, read := NormalizeClassifierAnswer(call.response, firstLabels(dictated, key.system, key.user))
			if !read {
				label = ""
			}
			answered = append(answered, label)
		}
		slices.Sort(submitted)
		slices.Sort(answered)
		if !slices.Equal(submitted, answered) {
			return WitnessResult{Verdict: WitnessLabelsContradicted}
		}
	}
	return WitnessResult{Verdict: WitnessRaised, RequestIDs: requestIDs, Model: commonest(models), Models: sortedModels(models), QuerySources: sortedModels(boolKeys(sources))}
}

func labelOrNone(label *string) string {
	if label == nil {
		return ""
	}
	return *label
}

func firstLabels(dictated []dictatedCall, system, user string) []string {
	for _, call := range dictated {
		if call.system == system && call.user == user {
			return call.labels
		}
	}
	return nil
}

func commonest(counts map[string]int) string {
	best, bestCount := "", 0
	for model, count := range counts {
		if count > bestCount || (count == bestCount && model < best) {
			best, bestCount = model, count
		}
	}
	return best
}

func sortedModels(counts map[string]int) []string {
	models := make([]string, 0, len(counts))
	for model := range counts {
		models = append(models, model)
	}
	slices.Sort(models)
	return models
}

// String renders a result for the journal.
func (r WitnessResult) String() string {
	return string(r.Verdict) + ":" + r.Reason + ":" + strconv.Itoa(len(r.RequestIDs))
}

func boolKeys(set map[string]bool) map[string]int {
	counts := make(map[string]int, len(set))
	for key := range set {
		counts[key] = 1
	}
	return counts
}
