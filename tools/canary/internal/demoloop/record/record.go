// Package record is the recording of one run of the agent loop: the names of its
// files and the shapes the runner writes and the renderer reads. A recording is
// a directory; nothing in it holds the credential of the session.
package record

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
)

// Files of a recording.
const (
	// MetaFile describes the run: what it was asked, how it was launched and how
	// it ended.
	MetaFile = "run.json"
	// StepsFile holds the outcome of each preparation step, one JSON line each.
	StepsFile = "setup.jsonl"
	// StreamFile is the event stream of the session, as the CLI printed it.
	StreamFile = "stream.jsonl"
	// TimesFile holds the time each line of the stream arrived.
	TimesFile = "stream-times.jsonl"
	// StderrFile is what the CLI printed to its error stream.
	StderrFile = "stderr.txt"
	// HooksFile holds every hook call the recording shims saw.
	HooksFile = evidence.HooksFile
	// InitialGateFile is the gate's JSON report on the equipped project, before the session.
	InitialGateFile = "gate-before.json"
	// FinalGateFile is the gate's JSON report on the project the session left.
	FinalGateFile = "gate-after.json"
	// ChangeFile is the change the session made, as a unified diff against the
	// equipped commit.
	ChangeFile = "change.diff"
	// MutantsDir holds the edit set of every mutant the discriminating-power
	// module kept, by mutant id.
	MutantsDir = "mutants"
	// TranscriptMarkdown and TranscriptHTML are the rendered transcript.
	TranscriptMarkdown = "transcript.md"
	TranscriptHTML     = "transcript.html"
)

// Step is the outcome of one preparation command.
type Step struct {
	Argv     []string `json:"argv"`
	Purpose  string   `json:"purpose"`
	ExitCode int      `json:"exitCode"`
	Stdout   string   `json:"stdout,omitempty"`
	Stderr   string   `json:"stderr,omitempty"`
}

// Meta describes one run.
type Meta struct {
	Task       string `json:"task"`
	Arm        string `json:"arm"`
	Repetition int    `json:"repetition"`
	Model      string `json:"model"`
	Prompt     string `json:"prompt"`
	// Argv is the session's command line. It never holds the credential.
	Argv []string `json:"argv"`
	// Caps are the bounds the runner enforced.
	EventCap       int    `json:"eventCap"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	BudgetUSD      string `json:"budgetUsd"`
	// Versions maps each binary of the run to the version it reported.
	Versions  map[string]string `json:"versions"`
	StartedAt time.Time         `json:"startedAt"`
	EndedAt   time.Time         `json:"endedAt"`
	ExitCode  int               `json:"exitCode"`
	// Killed is why the runner ended the session, or empty.
	Killed string `json:"killed,omitempty"`
}

// Arrival is the time one stream line arrived, after the session started.
type Arrival struct {
	Line   int   `json:"line"`
	Millis int64 `json:"ms"`
}

// DecodeArrivals reads the times file.
func DecodeArrivals(data []byte) ([]Arrival, error) {
	lines, err := evidence.Lines(TimesFile, data)
	if err != nil {
		return nil, err
	}
	arrivals := make([]Arrival, 0, len(lines))
	for _, line := range lines {
		var arrival Arrival
		if err := json.Unmarshal(line.Raw, &arrival); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", TimesFile, line.Number, err)
		}
		arrivals = append(arrivals, arrival)
	}
	return arrivals, nil
}

// DecodeSteps reads the steps file.
func DecodeSteps(data []byte) ([]Step, error) {
	lines, err := evidence.Lines(StepsFile, data)
	if err != nil {
		return nil, err
	}
	steps := make([]Step, 0, len(lines))
	for _, line := range lines {
		var step Step
		if err := json.Unmarshal(line.Raw, &step); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", StepsFile, line.Number, err)
		}
		steps = append(steps, step)
	}
	return steps, nil
}
