package app

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/checks"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

// ReportFile is the file the verdicts of a run are written to, in its records.
const ReportFile = "report.json"

// Load reads the recorded session of a records directory.
func (r Runner) Load(recordsDir string) (evidence.Evidence, error) {
	read := func(name string) ([]byte, error) { return r.Files.Read(filepath.Join(recordsDir, name)) }
	var files evidence.Files
	targets := []struct {
		name string
		into *[]byte
	}{
		{evidence.SessionFile, &files.Session}, {evidence.StreamFile, &files.Stream}, {evidence.HooksFile, &files.Hooks},
		{evidence.JournalFile, &files.Journal}, {evidence.ClassifyJournalFile, &files.ClassifyJournal},
		{evidence.NoCallFile, &files.NoCall}, {evidence.EditedFile, &files.Edited},
		{evidence.SandboxProbeFile, &files.SandboxProbe}, {evidence.TouchedFile, &files.Touched},
		{evidence.RolloutMainFile, &files.RolloutMain},
	}
	for _, target := range targets {
		data, err := read(target.name)
		if err != nil {
			return evidence.Evidence{}, fmt.Errorf("%s: %w", target.name, err)
		}
		*target.into = data
	}
	var err error
	if files.RolloutChildren, err = r.loadRolloutChildren(recordsDir); err != nil {
		return evidence.Evidence{}, err
	}
	return evidence.Assemble(files)
}

// loadRolloutChildren reads the rollouts of the spawned threads a Codex session
// left in its records, keyed by file name.
func (r Runner) loadRolloutChildren(recordsDir string) (map[string][]byte, error) {
	paths, err := r.Files.Glob(filepath.Join(recordsDir, evidence.RolloutChildrenDir, "*"))
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	children := make(map[string][]byte, len(paths))
	for _, path := range paths {
		data, err := r.Files.Read(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Join(evidence.RolloutChildrenDir, filepath.Base(path)), err)
		}
		children[filepath.Base(path)] = data
	}
	return children, nil
}

// Evaluate reads a recorded session and concludes every check of the harness. It
// makes no model call and starts no process, so a recorded session can be
// evaluated again whenever a check changes.
func (r Runner) Evaluate(recordsDir, harnessFlag string) (verdict.Report, error) {
	found, err := r.Load(recordsDir)
	if err != nil {
		return verdict.Report{}, err
	}
	report, err := checks.Evaluate(harnessFlag, found)
	if err != nil {
		return verdict.Report{}, err
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return verdict.Report{}, err
	}
	return report, r.Files.Write(filepath.Join(recordsDir, ReportFile), append(encoded, '\n'), false)
}

// WriteReport prints one JSON verdict per line, and a one-line summary to the
// error stream.
func WriteReport(report verdict.Report, stdout, stderr io.Writer) error {
	encoder := json.NewEncoder(stdout)
	for _, v := range report.Verdicts {
		if err := encoder.Encode(v); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(stderr, "%s: %d PASS, %d FAIL, %d INCONCLUSIVE\n", report.Harness,
		report.Count(verdict.Pass), report.Count(verdict.Fail), report.Count(verdict.Inconclusive))
	return err
}
