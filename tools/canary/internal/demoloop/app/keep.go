package app

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
)

// Placeholders a kept recording writes in place of the machine's own paths.
const (
	PlaceholderRun  = "<run>"
	PlaceholderRepo = "<repo>"
	PlaceholderHome = "~"
	// privateRoot is the prefix macOS resolves its temporary directories under.
	privateRoot = "/private"
)

// recordingPattern matches the files of the records a recording keeps.
const recordingPattern = "*"

// ErrRecordingExists refuses to write a recording over another.
var ErrRecordingExists = errors.New("a recording is already there")

// Keeper copies the records of a run into a recording meant to be committed.
type Keeper struct {
	Files canary.Files
}

// Keep copies the records of the run directory into the destination, with the
// run directory, the repository root and the home directory written as
// placeholders, so that a recording names no path of the machine it was made on.
// It refuses a destination that already holds a recording, and records that hold
// the token, given exactly, or any text of a token's shape.
func (k Keeper) Keep(run, repo, home, destination, token string) error {
	if k.Files.Exists(filepath.Join(destination, record.MetaFile)) {
		return fmt.Errorf("%w: %s; remove it first", ErrRecordingExists, destination)
	}
	records := filepath.Join(run, plan.RecordsDir)
	if !k.Files.Exists(filepath.Join(records, record.MetaFile)) {
		return fmt.Errorf("%w: %s has no %s", ErrNoRecording, records, record.MetaFile)
	}
	files, err := recordFiles(k.Files, records)
	if err != nil {
		return err
	}
	if hits := record.TokenHits(files, token); len(hits) > 0 {
		return fmt.Errorf("%w: %s; nothing was kept", ErrTokenInRecord, strings.Join(hits, ", "))
	}
	replacements := placeholders(run, repo, home)
	paths, err := k.Files.Glob(filepath.Join(records, recordingPattern))
	if err != nil {
		return err
	}
	for _, path := range paths {
		content, err := k.Files.Read(path)
		if err != nil || content == nil {
			continue
		}
		if err := k.Files.Write(filepath.Join(destination, filepath.Base(path)), redact(content, replacements), false); err != nil {
			return err
		}
	}
	return nil
}

type replacement struct{ from, to []byte }

// placeholders orders the replacements from the longest path down, each with
// its form outside /private, so that a path inside another keeps its own name.
func placeholders(run, repo, home string) []replacement {
	var found []replacement
	for _, pair := range [][2]string{{run, PlaceholderRun}, {repo, PlaceholderRepo}, {home, PlaceholderHome}} {
		if pair[0] == "" {
			continue
		}
		found = append(found, replacement{[]byte(pair[0]), []byte(pair[1])})
		if trimmed, trimmedFound := strings.CutPrefix(pair[0], privateRoot); trimmedFound {
			found = append(found, replacement{[]byte(trimmed), []byte(pair[1])})
		}
	}
	return found
}

func redact(content []byte, replacements []replacement) []byte {
	for _, r := range replacements {
		content = bytes.ReplaceAll(content, r.from, r.to)
	}
	return content
}
