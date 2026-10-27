package analyze

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	demoapp "github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The files and directories of results/ (PREREG §11.1).
const (
	ResultsDir  = "results"
	SummaryFile = "summary.json"
	ReportFile  = "REPORT.md"
	SumsFile    = "SHA256SUMS"
	RunsDir     = "runs"
	RawDir      = "raw"
	fileMode    = 0o644
	dirMode     = 0o755
)

// keptRecords are the records of a run that results/runs/<run>/ holds.
var keptRecords = []string{
	record.MetaFile, record.StepsFile, record.InitialGateFile, record.FinalGateFile, record.ChangeFile,
	study.OracleFile, study.HeldOutFile, study.DetectorsFile, study.FactsFile,
}

// rawRecords are the records of a run that results/runs/<run>/raw/ holds; the
// public history leaves them out and SHA256SUMS covers them.
var rawRecords = []string{record.StreamFile, record.TimesFile, record.StderrFile, record.HooksFile, study.StopsDir, record.EgressFile}

// renderedFrom are the records the transcript is rendered from.
var renderedFrom = []string{record.MetaFile, record.StreamFile, record.TimesFile, record.HooksFile, record.FinalGateFile, record.ChangeFile}

// Paths are the machine's paths the results write as placeholders.
type Paths struct {
	Repo, Home string
}

func (p Paths) redact(content []byte, run string) []byte {
	return demoapp.RedactPaths(content, run, p.Repo, p.Home)
}

// CountedRecords is one counted attempt's records, to be kept under its run's
// name.
type CountedRecords struct {
	Run string
	// Attempt is the attempt's directory, written as <run>; Records its records.
	Attempt, Records string
}

// writeRun copies a counted attempt's records into results/runs/<run>/ with the
// machine's paths as placeholders, renders its transcript there, and returns the
// raw files written, relative to the results directory.
func writeRun(results string, counted CountedRecords, paths Paths) ([]string, error) {
	destination := filepath.Join(results, RunsDir, counted.Run)
	copyOne := func(from, to string) error {
		content, err := os.ReadFile(from)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		return writeFile(to, paths.redact(content, counted.Attempt))
	}
	for _, name := range keptRecords {
		if err := copyOne(filepath.Join(counted.Records, name), filepath.Join(destination, name)); err != nil {
			return nil, err
		}
	}
	var raw []string
	for _, name := range rawRecords {
		err := filepath.WalkDir(filepath.Join(counted.Records, name), func(path string, entry fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			if err != nil || entry.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(counted.Records, path)
			to := filepath.Join(destination, RawDir, rel)
			raw = append(raw, filepath.ToSlash(filepath.Join(RunsDir, counted.Run, RawDir, rel)))
			return copyOne(path, to)
		})
		if err != nil {
			return nil, err
		}
	}
	return raw, renderTranscript(counted, destination, paths)
}

// renderTranscript renders the transcript of the redacted records into the
// run's results directory.
func renderTranscript(counted CountedRecords, destination string, paths Paths) error {
	staging, err := os.MkdirTemp("", "loop1-transcript-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	for _, name := range renderedFrom {
		content, err := os.ReadFile(filepath.Join(counted.Records, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := writeFile(filepath.Join(staging, name), paths.redact(content, counted.Attempt)); err != nil {
			return err
		}
	}
	return demoapp.Renderer{Files: adapters.OSFiles{}}.Write(staging, destination, false)
}

func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}
	return os.WriteFile(path, content, fileMode)
}

// copyDeviations copies the deviation log and its timestamps into results/.
func copyDeviations(results string, layout study.Layout) error {
	if content, err := os.ReadFile(layout.Deviations()); err == nil {
		if err := writeFile(filepath.Join(results, study.DeviationsFile), content); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entries, err := os.ReadDir(layout.Timestamps())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(layout.Timestamps(), entry.Name()))
		if err != nil {
			return err
		}
		if err := writeFile(filepath.Join(results, study.DeviationTimestampsDir, entry.Name()), content); err != nil {
			return err
		}
	}
	return nil
}

// writeSums writes SHA256SUMS over the raw files, in the format of shasum -a 256.
func writeSums(results string, raw []string) error {
	slices.Sort(raw)
	var sums strings.Builder
	for _, rel := range raw {
		content, err := os.ReadFile(filepath.Join(results, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		digest := sha256.Sum256(content)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(digest[:]), rel)
	}
	return writeFile(filepath.Join(results, SumsFile), []byte(sums.String()))
}
