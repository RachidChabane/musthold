// Package evidence is what one canary session left behind, decoded: the event
// stream of the harness, the payloads the module's hooks received and the
// decisions they gave, the module's journal, and the results of the probes that
// make no model call. Every decoder keeps what it does not know, because the
// formats of the harnesses are only partly documented; a check reads only the
// members it needs and treats an absent one as no evidence.
package evidence

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

// Names of the files a recorded session consists of, relative to its directory.
const (
	SessionFile = "session.json"
	StreamFile  = "stream.jsonl"
	StderrFile  = "stderr.txt"
	HooksFile   = "hooks.jsonl"
	JournalFile = "journal.jsonl"
	// ClassifyJournalFile is the journal of the module that classifies in shadow.
	ClassifyJournalFile = "classify-journal.jsonl"
	NoCallFile          = "nocall.json"
	EditedFile          = "edited.txt"
	SandboxProbeFile    = "sandbox-probe.json"
	TouchedFile         = "touched.json"
)

// maxLineBytes bounds one line of a recorded JSON-lines file.
const maxLineBytes = 16 << 20

// Files holds the raw bytes of the recorded files; one that was not recorded is
// empty.
type Files struct {
	Session, Stream, Hooks, Journal, ClassifyJournal, NoCall, Edited, SandboxProbe, Touched []byte
	// RolloutMain is the rollout of a Codex session's main thread, and
	// RolloutChildren those of the threads it spawned, keyed by file name.
	RolloutMain     []byte
	RolloutChildren map[string][]byte
}

// Line is one line of a JSON-lines file with its 1-based number.
type Line struct {
	Number int
	Raw    json.RawMessage
}

// Evidence is a recorded session, decoded.
type Evidence struct {
	Session Session
	Stream  []Line
	Hooks   []HookCall
	Journal []JournalEntry
	// ClassifyJournal is the journal of the module that classifies in shadow.
	ClassifyJournal []JournalEntry
	NoCall          NoCall
	Edited          string
	SandboxProbe    SandboxProbe
	Touched         Touched
	Rollouts        Rollouts
}

// JournalEntry is one record of the module's journal with its line number.
type JournalEntry struct {
	Line int
	hostproto.Record
}

// Assemble decodes the recorded files. A line that is not JSON is an error that
// names the file and the line: a corrupt record must not read as absent evidence.
func Assemble(files Files) (Evidence, error) {
	var found Evidence
	if len(bytes.TrimSpace(files.Session)) > 0 {
		if err := json.Unmarshal(files.Session, &found.Session); err != nil {
			return Evidence{}, fmt.Errorf("%s: %w", SessionFile, err)
		}
	}
	var err error
	if found.Stream, err = Lines(StreamFile, files.Stream); err != nil {
		return Evidence{}, err
	}
	if found.Hooks, err = DecodeHooks(files.Hooks); err != nil {
		return Evidence{}, err
	}
	if found.Journal, err = decodeJournal(files.Journal); err != nil {
		return Evidence{}, err
	}
	if found.ClassifyJournal, err = decodeJournalFile(ClassifyJournalFile, files.ClassifyJournal); err != nil {
		return Evidence{}, err
	}
	if len(bytes.TrimSpace(files.NoCall)) > 0 {
		if err := json.Unmarshal(files.NoCall, &found.NoCall); err != nil {
			return Evidence{}, fmt.Errorf("%s: %w", NoCallFile, err)
		}
	}
	if len(bytes.TrimSpace(files.SandboxProbe)) > 0 {
		if err := json.Unmarshal(files.SandboxProbe, &found.SandboxProbe); err != nil {
			return Evidence{}, fmt.Errorf("%s: %w", SandboxProbeFile, err)
		}
	}
	if len(bytes.TrimSpace(files.Touched)) > 0 {
		if err := json.Unmarshal(files.Touched, &found.Touched); err != nil {
			return Evidence{}, fmt.Errorf("%s: %w", TouchedFile, err)
		}
	}
	if found.Rollouts, err = decodeRollouts(files.RolloutMain, files.RolloutChildren); err != nil {
		return Evidence{}, err
	}
	found.Edited = string(files.Edited)
	return found, nil
}

// Lines splits a JSON-lines file into its non-blank lines.
func Lines(name string, data []byte) ([]Line, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(nil, maxLineBytes)
	var lines []Line
	for number := 1; scanner.Scan(); number++ {
		text := bytes.TrimSpace(scanner.Bytes())
		if len(text) == 0 {
			continue
		}
		if !json.Valid(text) {
			return nil, fmt.Errorf("%s:%d: not JSON", name, number)
		}
		lines = append(lines, Line{Number: number, Raw: append(json.RawMessage(nil), text...)})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return lines, nil
}

func decodeJournal(data []byte) ([]JournalEntry, error) {
	return decodeJournalFile(JournalFile, data)
}

func decodeJournalFile(name string, data []byte) ([]JournalEntry, error) {
	lines, err := Lines(name, data)
	if err != nil {
		return nil, err
	}
	entries := make([]JournalEntry, 0, len(lines))
	for _, line := range lines {
		entry := JournalEntry{Line: line.Number}
		if err := json.Unmarshal(line.Raw, &entry.Record); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", name, line.Number, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Events returns the journal entries of a kind.
func (e Evidence) Events(kind hostproto.EventKind) []JournalEntry {
	return entriesOf(e.Journal, kind)
}

// ClassifyEvents returns the entries of a kind in the journal of the module that
// classifies in shadow.
func (e Evidence) ClassifyEvents(kind hostproto.EventKind) []JournalEntry {
	return entriesOf(e.ClassifyJournal, kind)
}

func entriesOf(journal []JournalEntry, kind hostproto.EventKind) []JournalEntry {
	var found []JournalEntry
	for _, entry := range journal {
		if entry.Event.Kind == kind {
			found = append(found, entry)
		}
	}
	return found
}

// At points at a line of a recorded file.
func At(file string, line int, note string) verdict.Pointer {
	return verdict.Pointer{File: file, Line: line, Note: note}
}

// StreamAt points at a line of the harness's event stream.
func StreamAt(line int, note string) verdict.Pointer { return At(StreamFile, line, note) }

// HooksAt points at a record of the hook log.
func HooksAt(line int, note string) verdict.Pointer { return At(HooksFile, line, note) }

// JournalAt points at a record of the module's journal.
func JournalAt(line int, note string) verdict.Pointer { return At(JournalFile, line, note) }

// ClassifyJournalAt points at a record of the journal of the module that
// classifies in shadow.
func ClassifyJournalAt(line int, note string) verdict.Pointer {
	return At(ClassifyJournalFile, line, note)
}
