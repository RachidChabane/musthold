package hoststore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	lineEnd = '\n'
	// maxJournalLine bounds one journal line when the file is scanned.
	maxJournalLine = 1 << 20
)

// Append implements hostproto.Journal: under the lock it reads the head of the
// journal, chains the event after it and appends the record with O_APPEND.
func (s *Store) Append(event hostproto.Event) (hostproto.Record, error) {
	return withLock(s, func() (hostproto.Record, error) {
		head, err := s.readHead()
		if err != nil {
			return hostproto.Record{}, err
		}
		record, err := hostproto.Chain(head, event)
		if err != nil {
			return hostproto.Record{}, err
		}
		return record, s.appendLine(record)
	})
}

func (s *Store) appendLine(record hostproto.Record) error {
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode journal record: %w", err)
	}
	file, err := os.OpenFile(s.path(JournalFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, FileMode)
	if err != nil {
		return fmt.Errorf("open journal: %w", err)
	}
	_, writeErr := file.Write(append(line, lineEnd))
	var syncErr error
	if writeErr == nil {
		syncErr = file.Sync()
	}
	if err := errors.Join(writeErr, syncErr, file.Close()); err != nil {
		return fmt.Errorf("append to journal: %w", err)
	}
	return nil
}

// Records implements hostproto.Journal.
func (s *Store) Records() ([]hostproto.Record, error) {
	return withLock(s, func() ([]hostproto.Record, error) {
		records := []hostproto.Record{}
		err := s.scanJournal(func(line []byte) error {
			record, err := decodeRecord(line)
			records = append(records, record)
			return err
		})
		return records, err
	})
}

// readHead returns the last record of the journal, or nil for an empty one.
func (s *Store) readHead() (*hostproto.Record, error) {
	var lastLine []byte
	err := s.scanJournal(func(line []byte) error {
		lastLine = bytes.Clone(line)
		return nil
	})
	if err != nil || lastLine == nil {
		return nil, err
	}
	record, err := decodeRecord(lastLine)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// scanJournal calls visit with every non-empty line of the journal.
func (s *Store) scanJournal(visit func(line []byte) error) error {
	file, err := os.Open(s.path(JournalFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open journal: %w", err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, maxJournalLine)
	for scanner.Scan() {
		if line := scanner.Bytes(); len(line) > 0 {
			if err := visit(line); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read journal: %w", err)
	}
	return nil
}

func decodeRecord(line []byte) (hostproto.Record, error) {
	var record hostproto.Record
	if err := json.Unmarshal(line, &record); err != nil {
		return record, fmt.Errorf("decode journal record: %w", err)
	}
	return record, nil
}
