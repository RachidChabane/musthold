package node

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// The files a run of a scan leaves in its record directory, which the shipped
// agent, reporter and global setup write.
const (
	recordSuffix        = ".jsonl"
	reporterRecord      = "reporter.jsonl"
	globalSetupFailure  = "global-setup.failed"
	recordLineMaxBytes  = 1 << 30
	recordLineStartSize = 1 << 16
)

// The kinds of line of a record.
const (
	lineProcess      = "process"
	lineCapabilities = "capabilities"
	lineScript       = "script"
	lineTake         = "take"
	lineTest         = "test"
	lineFile         = "file"
	lineResult       = "result"
)

// What a take marks, as the agent names it: the takes of the start and the
// end of a process mark nothing the reading of the record needs.
const (
	takeFile       = "file"
	takeOpen       = "open"
	takeClose      = "close"
	takeSliceStart = "sliceStart"
	takeSliceEnd   = "sliceEnd"
)

// Outcomes the shipped environment and runner give a test.
const (
	outcomePassed  = "passed"
	outcomeFailed  = "failed"
	outcomeErrored = "errored"
	outcomeSkipped = "skipped"
)

// capabilities are the preconditions of the ownership rule a process showed
// (see ownership); a false one gives its fallback.
type capabilities struct {
	// Inspector: an inspector session of the process takes V8 precise
	// coverage. Without it the process records its lines through
	// NODE_V8_COVERAGE and owns nothing.
	Inspector bool `json:"inspector"`
	// SynchronousTake: a take answers before the call returns, inside a
	// runner's event or hook. Without it the window opens at the test
	// function, so beforeEach is not owned, and no slice is cut: the window
	// is read against hollow runs of the test, as without AsyncHooks.
	SynchronousTake bool `json:"synchronousTake"`
	// AsyncHooks: the async hooks see the promises of the test realm, so the
	// callbacks of foreign origin are cut out of the window. Without them the
	// window is read against hollow runs of the test.
	AsyncHooks bool `json:"asyncHooks"`
	// Evaluation: a take shows a module evaluated in the window. Without it
	// the process owns nothing.
	Evaluation bool `json:"evaluation"`
	// FakeTimers: the pending fake timers of a realm can be enumerated, so a
	// timer created before the window is cut out of it. Without it a test
	// whose realm fakes timers when its window opens owns nothing.
	FakeTimers bool `json:"fakeTimers"`
	// Maps: the generated code of each module and the source map it carries
	// are captured. Without them only a module identical to its file maps.
	Maps bool `json:"maps"`
	// Bounds: the window holds the start and the settling of the test
	// function. A window that does not owns nothing.
	Bounds bool `json:"bounds"`
	// Retention: no function's counts vanish between takes. Without it the
	// process owns nothing.
	Retention bool `json:"retention"`
}

// and keeps the capabilities both show.
func (c capabilities) and(other capabilities) capabilities {
	return capabilities{
		Inspector: c.Inspector && other.Inspector, SynchronousTake: c.SynchronousTake && other.SynchronousTake,
		AsyncHooks: c.AsyncHooks && other.AsyncHooks, Evaluation: c.Evaluation && other.Evaluation,
		FakeTimers: c.FakeTimers && other.FakeTimers, Maps: c.Maps && other.Maps,
		Bounds: c.Bounds && other.Bounds, Retention: c.Retention && other.Retention,
	}
}

// ranges are the counts of one take, by script id: for each function, its
// ranges as [start, end, count], the first range the function's own, offsets
// in UTF-16 code units of the script's source.
type ranges map[string][][][3]int

// recordLine is one line of a record, whichever its kind.
type recordLine struct {
	Kind         string        `json:"k"`
	Recording    bool          `json:"recording"`
	Cumulative   bool          `json:"cumulative"`
	Capabilities *capabilities `json:"capabilities"`
	// A script.
	ID     string `json:"id"`
	File   string `json:"file"`
	Source string `json:"source"`
	Map    string `json:"map"`
	Prefix int    `json:"prefix"`
	// A take.
	At         string `json:"at"`
	TakenFile  string `json:"f"`
	Counts     ranges `json:"c"`
	Bounded    bool   `json:"bounded"`
	Callbacks  bool   `json:"callbacks"`
	FakeUnseen bool   `json:"fakeUnseen"`
	// A test.
	Outcome string `json:"outcome"`
	// A result of the reporter.
	FailedToRun bool             `json:"failedToRun"`
	Tests       []reportedResult `json:"tests"`
}

// reportedResult is the status the reporter gives a test.
type reportedResult struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Invocations int    `json:"invocations"`
}

// scriptRecord is a module of the project a process ran.
type scriptRecord struct {
	file, source, sourceMap string
	prefix                  int
}

// takeRecord is one take of a process: the counts since the previous one and
// what it marks.
type takeRecord struct {
	at, file                       string
	counts                         ranges
	bounded, callbacks, fakeUnseen bool
}

// testRecord is the outcome a process gave a test.
type testRecord struct {
	id      ecosystem.TestID
	file    string
	outcome string
}

// processRecord is what one process of a run recorded.
type processRecord struct {
	recording bool
	// cumulative is whether a take counts from the start of the process
	// rather than from the previous take.
	cumulative   bool
	capabilities capabilities
	scripts      map[string]scriptRecord
	takes        []takeRecord
	tests        []testRecord
	failedFiles  []string
}

// runRecord is what every process of one run of the runner left: the records
// of the processes that ran tests, the reporter's results and whether the
// global setup failed.
type runRecord struct {
	processes         []processRecord
	reported          []recordLine
	globalSetupFailed bool
}

// readRunRecord reads the record directory of one run; a line that cannot be
// read ends the record of its process there.
func readRunRecord(dir string) (runRecord, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return runRecord{}, nil
	}
	if err != nil {
		return runRecord{}, fmt.Errorf("read the scan record %s: %w", dir, err)
	}
	var record runRecord
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	for _, name := range names {
		switch {
		case name == globalSetupFailure:
			record.globalSetupFailed = true
		case name == reporterRecord:
			lines, err := readLines(filepath.Join(dir, name))
			if err != nil {
				return runRecord{}, err
			}
			for _, line := range lines {
				if line.Kind != lineResult {
					continue
				}
				record.reported = append(record.reported, line)
			}
		case strings.HasSuffix(name, recordSuffix):
			lines, err := readLines(filepath.Join(dir, name))
			if err != nil {
				return runRecord{}, err
			}
			if process, ok := processOf(lines); ok {
				record.processes = append(record.processes, process)
			}
		}
	}
	return record, nil
}

// readLines decodes the complete lines of a record file; a line cut short by
// a killed process, and every line after it, is dropped.
func readLines(file string) ([]recordLine, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read the scan record %s: %w", file, err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, recordLineStartSize), recordLineMaxBytes)
	var lines []recordLine
	for scanner.Scan() {
		var line recordLine
		if json.Unmarshal(scanner.Bytes(), &line) != nil {
			break
		}
		lines = append(lines, line)
	}
	return lines, nil
}

// processOf assembles the record of a process from its lines; a file with no
// process line is no record.
func processOf(lines []recordLine) (processRecord, bool) {
	if len(lines) == 0 || lines[0].Kind != lineProcess || lines[0].Capabilities == nil {
		return processRecord{}, false
	}
	process := processRecord{recording: lines[0].Recording, cumulative: lines[0].Cumulative, capabilities: *lines[0].Capabilities, scripts: map[string]scriptRecord{}}
	checked := false
	for _, line := range lines[1:] {
		switch line.Kind {
		case lineCapabilities:
			if line.Capabilities == nil {
				continue
			}
			if !checked {
				process.capabilities = *line.Capabilities
				checked = true
				continue
			}
			process.capabilities = process.capabilities.and(*line.Capabilities)
		case lineScript:
			process.scripts[line.ID] = scriptRecord{file: line.File, source: line.Source, sourceMap: line.Map, prefix: line.Prefix}
		case lineTake:
			process.takes = append(process.takes, takeRecord{
				at: line.At, file: line.TakenFile, counts: line.Counts,
				bounded: line.Bounded, callbacks: line.Callbacks, fakeUnseen: line.FakeUnseen,
			})
		case lineTest:
			process.tests = append(process.tests, testRecord{id: ecosystem.TestID(line.ID), file: line.File, outcome: line.Outcome})
		case lineFile:
			process.failedFiles = append(process.failedFiles, line.File)
		}
	}
	if !checked && process.recording {
		// A recording whose test realm was never checked shows none of the
		// realm's preconditions.
		process.capabilities.AsyncHooks, process.capabilities.Evaluation = false, false
	}
	return process, true
}
