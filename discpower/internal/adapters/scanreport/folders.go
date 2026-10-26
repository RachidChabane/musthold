package scanreport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/scan"
)

// File modes of the reports.
const (
	directoryMode  = 0o755
	fileMode       = 0o644
	executableMode = 0o755
	jsonIndent     = "  "
	shellQuote     = "'"
	escapedQuote   = `'\''`
)

// File is one file of a scan's reports, relative to the work tree.
type File struct {
	Path       string
	Content    []byte
	Executable bool
}

// Files lays out the reports of a scan: the Markdown report, the statement,
// and one folder per finding with the patch, the reproduction script, the
// killers of its routes and the record a replay reads, and one folder per
// broken promise.
func Files(report scan.Report) ([]File, error) {
	dir := path.Join(Directory, ShortCommit(report.Commit))
	files := []File{
		{Path: path.Join(dir, reportFile), Content: []byte(Markdown(report))},
		{Path: path.Join(dir, statement), Content: report.Encoded},
	}
	for _, finding := range report.Findings {
		folder := FindingDir(report, finding)
		record, err := encodeRecord(scan.RecordOf(report, finding))
		if err != nil {
			return nil, fmt.Errorf("encode finding %d: %w", finding.Rank, err)
		}
		files = append(files,
			File{Path: path.Join(folder, patchFile), Content: []byte(patchOf(finding.Files))},
			File{Path: path.Join(folder, reproFile), Content: []byte(ReproScript(report, finding)), Executable: true},
			File{Path: path.Join(folder, findingFile), Content: record},
		)
		files = append(files, killerFiles(report, folder, finding.Killers, findingScript)...)
	}
	return appendBrokenPromises(files, report)
}

// appendBrokenPromises adds the folder of each broken promise: the patch, the
// script that runs its witness on the mutant, the witness file, the killers
// and the record a replay reads.
func appendBrokenPromises(files []File, report scan.Report) ([]File, error) {
	if report.Promises == nil {
		return files, nil
	}
	for _, broken := range report.Promises.Broken {
		folder := BrokenPromiseDir(report, broken)
		record, err := encodeRecord(scan.BrokenRecordOf(report, broken))
		if err != nil {
			return nil, fmt.Errorf("encode broken promise %d: %w", broken.Rank, err)
		}
		files = append(files,
			File{Path: path.Join(folder, patchFile), Content: []byte(patchOf(broken.Files))},
			File{Path: path.Join(folder, findingFile), Content: record},
		)
		files = append(files, killerFiles(report, folder, []scan.Killer{broken.Killer}, brokenScriptName)...)
	}
	return files, nil
}

// patchOf is the unified diff of the files a mutant changes.
func patchOf(changes []scan.FileChange) string {
	var patch strings.Builder
	for _, file := range changes {
		patch.WriteString(UnifiedDiff(file.Path, file.Before, file.After))
	}
	return patch.String()
}

// shebang opens the scripts of a folder.
const shebang = "#!/bin/sh"

// ReproScript checks out the scanned commit into a temporary directory, applies
// the patch and runs the one test; its exit status is the test's.
func ReproScript(report scan.Report, finding scan.Finding) string {
	lines := []string{
		shebang,
		fmt.Sprintf("# Runs %s on %s with mutant.patch applied; exit status 0 means the test still passes.", finding.Test, ShortCommit(report.Commit)),
	}
	lines = append(lines, checkoutLines(report)...)
	return strings.Join(append(lines, commandLine(finding.Repro), ""), newline)
}

// checkoutLines are the lines every script of a folder starts its work with:
// the commit checked out into a temporary directory, its dependencies linked
// and the patch of the folder applied.
func checkoutLines(report scan.Report) []string {
	return []string{
		"set -eu",
		`finding="$(cd "$(dirname "$0")" && pwd)"`,
		`root="$(git -C "$finding" rev-parse --show-toplevel)"`,
		`work="$(mktemp -d)"`,
		`trap 'rm -rf "$work"' EXIT`,
		fmt.Sprintf(`git -C "$root" archive %s | tar -x -C "$work"`, report.Commit),
		`for dependencies in .venv node_modules; do`,
		`  if [ -e "$root/$dependencies" ]; then ln -s "$root/$dependencies" "$work/$dependencies"; fi`,
		`done`,
		`if [ -d "$work/.venv/bin" ]; then PATH="$work/.venv/bin:$PATH"; export PATH; fi`,
		`cd "$work"`,
		`git apply "$finding/mutant.patch"`,
	}
}

// encodeRecord writes a finding record as indented JSON that keeps source
// code as written.
func encodeRecord(record scan.FindingRecord) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", jsonIndent)
	if err := encoder.Encode(record); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func quote(argument string) string {
	return shellQuote + strings.ReplaceAll(argument, shellQuote, escapedQuote) + shellQuote
}

// Write writes the reports under the work tree, replacing the earlier reports
// of the same commit.
func Write(root string, report scan.Report, files []File) error {
	dir := filepath.Join(root, filepath.FromSlash(path.Join(Directory, ShortCommit(report.Commit))))
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clear %s: %w", dir, err)
	}
	for _, file := range files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), directoryMode); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
		}
		mode := os.FileMode(fileMode)
		if file.Executable {
			mode = executableMode
		}
		if err := os.WriteFile(target, file.Content, mode); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		if err := os.Chmod(target, mode); err != nil {
			return fmt.Errorf("set the mode of %s: %w", target, err)
		}
	}
	return nil
}
