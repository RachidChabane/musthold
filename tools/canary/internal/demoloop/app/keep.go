package app

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

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
// placeholders, so that a recording names no path of the machine it was made on,
// and renders the transcript of the copy.
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
	paths, err := k.Files.Glob(filepath.Join(records, recordingPattern))
	if err != nil {
		return err
	}
	for _, path := range paths {
		if slices.Contains(renderedFiles, filepath.Base(path)) {
			continue
		}
		content, err := k.Files.Read(path)
		if err != nil || content == nil {
			continue
		}
		if err := k.Files.Write(filepath.Join(destination, filepath.Base(path)), RedactPaths(content, run, repo, home), false); err != nil {
			return err
		}
	}
	return Renderer{Files: k.Files}.Write(destination, destination, false)
}

// renderedFiles are the records a recording renders from the others instead of
// copying: text substituted into rendered HTML would not be escaped.
var renderedFiles = []string{record.TranscriptMarkdown, record.TranscriptHTML}

type replacement struct{ from, to []byte }

// placeholders orders the replacements from the longest path down, each with
// its form outside /private, so that a path inside another keeps its own name;
// the dash-encoded forms follow, in the same order.
func placeholders(run, repo, home string) []replacement {
	var paths []replacement
	for _, pair := range [][2]string{{run, PlaceholderRun}, {repo, PlaceholderRepo}, {home, PlaceholderHome}} {
		if pair[0] == "" {
			continue
		}
		paths = append(paths, replacement{[]byte(pair[0]), []byte(pair[1])})
		if trimmed, trimmedFound := strings.CutPrefix(pair[0], privateRoot); trimmedFound {
			paths = append(paths, replacement{[]byte(trimmed), []byte(pair[1])})
		}
	}
	found := slices.Clone(paths)
	for _, path := range paths {
		found = append(found, replacement{[]byte(dashEncoded(string(path.from))), path.to})
	}
	return found
}

// dashEncoded is the name Claude Code gives a directory it names after a path:
// every character other than an ASCII letter or digit becomes a dash.
func dashEncoded(path string) string {
	return strings.Map(func(r rune) rune {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return r
		}
		return '-'
	}, path)
}

// RedactPaths writes the run directory, the repository root and the home
// directory of the content as the placeholders <run>, <repo> and ~, the longest
// path first, each also in its form outside /private and in its dash-encoded
// form.
func RedactPaths(content []byte, run, repo, home string) []byte {
	for _, r := range placeholders(run, repo, home) {
		content = bytes.ReplaceAll(content, r.from, r.to)
	}
	return content
}
