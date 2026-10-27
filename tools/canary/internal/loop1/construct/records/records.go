// Package records defines the construction records of the loop-1 study: every
// call to the task author and to the test writer, every output of an acceptance
// check and every dropped domain. They are appended to one JSON Lines file in
// the construction directory of the work directory, and read back whole.
package records

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// FileName is the name of the records file inside the construction directory.
const FileName = "records.jsonl"

// Role is who a call or a finding concerns.
type Role string

// The roles of construction.
const (
	// RoleAuthor writes the fixture, the reference, the violations, the refactors
	// and the task description.
	RoleAuthor Role = "author"
	// RoleOracle writes the tests of each criterion from the spec alone.
	RoleOracle Role = "oracle"
)

// Purpose says whether a call writes its role's files from nothing or repairs
// them after a failed check.
type Purpose string

// The purposes of a call.
const (
	PurposeGenerate Purpose = "generate"
	PurposeRepair   Purpose = "repair"
)

// Target is the part of a task that a finding is about.
type Target string

// The parts of a task.
const (
	TargetFixture   Target = "fixture"
	TargetReference Target = "reference"
	TargetVariants  Target = "variants"
	TargetTask      Target = "task"
	TargetOracle    Target = "oracle"
	TargetOutput    Target = "output"
)

// Call is one invocation of the model.
type Call struct {
	Role    Role    `json:"role"`
	Purpose Purpose `json:"purpose"`
	Domain  string  `json:"domain"`
	// Attempt counts the calls of this role for this domain from 1.
	Attempt      int       `json:"attempt"`
	PromptSHA256 string    `json:"prompt_sha256"`
	Schema       string    `json:"schema"`
	Output       string    `json:"output"`
	Stderr       string    `json:"stderr,omitempty"`
	CodexVersion string    `json:"codex_version"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	Exit         int       `json:"exit"`
	// Error is why the call produced no usable files.
	Error string `json:"error,omitempty"`
}

// Finding is one thing a failed check reports, written for the role it concerns.
type Finding struct {
	Role   Role   `json:"role"`
	Target Target `json:"target"`
	Text   string `json:"text"`
}

// Check is the outcome of one acceptance check on one round.
type Check struct {
	Domain string `json:"domain"`
	// Round counts the evaluations of this domain from 1.
	Round int    `json:"round"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	// Passed is whether the check's condition holds; Agreed whether its runs
	// agreed with one another.
	Passed   bool      `json:"passed"`
	Agreed   bool      `json:"agreed"`
	Findings []Finding `json:"findings,omitempty"`
	Output   string    `json:"output,omitempty"`
}

// Drop is a domain that failed acceptance after its repairs and was left out.
type Drop struct {
	Domain string    `json:"domain"`
	Check  string    `json:"check"`
	Reason string    `json:"reason"`
	Time   time.Time `json:"time"`
}

// Entry is one line of the records file: exactly one member is set.
type Entry struct {
	Call  *Call  `json:"call,omitempty"`
	Check *Check `json:"check,omitempty"`
	Drop  *Drop  `json:"drop,omitempty"`
}

// Log is everything the records file holds, in file order within each kind.
type Log struct {
	Calls  []Call  `json:"calls"`
	Checks []Check `json:"checks"`
	Drops  []Drop  `json:"drops"`
}

// Writer appends entries to the records.
type Writer interface {
	Append(entry Entry) error
}

// ErrMalformedRecords is returned for a records file that cannot be read.
var ErrMalformedRecords = errors.New("the construction records are malformed")

const (
	directoryMode = 0o755
	fileMode      = 0o644
	maxLineBytes  = 64 << 20
)

// Path is the records file of a work directory.
func Path(layout study.Layout) string {
	return filepath.Join(layout.Construction(), FileName)
}

// FileWriter appends entries to a records file, one JSON document per line.
type FileWriter struct {
	path string
	mu   sync.Mutex
}

// NewFileWriter writes the records file of the work directory.
func NewFileWriter(layout study.Layout) *FileWriter { return &FileWriter{path: Path(layout)} }

// Append adds one entry at the end of the file, creating it and its directory
// when they do not exist.
func (w *FileWriter) Append(entry Entry) error {
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode a construction record: %w", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(w.path), directoryMode); err != nil {
		return fmt.Errorf("create the construction directory: %w", err)
	}
	file, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, fileMode)
	if err != nil {
		return fmt.Errorf("open the construction records: %w", err)
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("append a construction record: %w", err)
	}
	return file.Close()
}

// Read parses a records file. A missing file is an empty log.
func Read(path string) (Log, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Log{}, nil
	}
	if err != nil {
		return Log{}, fmt.Errorf("read the construction records: %w", err)
	}
	return Parse(data)
}

// Parse reads the lines of a records file.
func Parse(data []byte) (Log, error) {
	var log Log
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(nil, maxLineBytes)
	for number := 1; scanner.Scan(); number++ {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var entry Entry
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&entry); err != nil {
			return Log{}, fmt.Errorf("%w: line %d: %v", ErrMalformedRecords, number, err)
		}
		switch {
		case entry.Call != nil:
			log.Calls = append(log.Calls, *entry.Call)
		case entry.Check != nil:
			log.Checks = append(log.Checks, *entry.Check)
		case entry.Drop != nil:
			log.Drops = append(log.Drops, *entry.Drop)
		default:
			return Log{}, fmt.Errorf("%w: line %d holds no record", ErrMalformedRecords, number)
		}
	}
	if err := scanner.Err(); err != nil {
		return Log{}, fmt.Errorf("%w: %v", ErrMalformedRecords, err)
	}
	return log, nil
}
