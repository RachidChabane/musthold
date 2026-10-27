// Package oracle is the oracle verb of the loop-1 study (PREREG §4): it judges the
// final change of every counted run against the task's oracle, and measures the
// strength of the run's test directories against the task's held-out violations.
//
// The package is split along the architecture. The pure rules (reading a JUnit
// report, the defect rule, the kill rule, the criterion of a test file) have no
// I/O. The adapters (the trees built on the host, the commands run through
// box.Runner, the repository archives read in the box) sit behind box.Runner and
// plain directories. The verb reads truth files and writes nothing into a run
// directory but records/oracle.json and records/heldout.json.
package oracle

import (
	"errors"
	"path/filepath"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The names of the study that the verb reads.
const (
	pytestIniFile  = "pytest.ini"
	testsDirName   = "tests"
	oracleDirName  = "oracle"
	repositoryDir  = ".git"
	configDir      = ".musthold"
	testResultsDir = "test-results"
	pycacheDir     = "__pycache__"
)

// The directories of the verb's own state, under the work directory.
const (
	verbDir  = "oracle"
	jobsDir  = "jobs"
	stateDir = "state"
)

// Errors of the verb.
var (
	ErrBox     = errors.New("the box could not run the command")
	ErrRecords = errors.New("the attempt's records cannot be read")
	ErrTruth   = errors.New("the task's truth files cannot be read")
	ErrArchive = errors.New("the repository archive cannot be read")
)

func recordsPath(runDir string) string { return filepath.Join(runDir, study.RecordsDir) }
func projectPath(runDir string) string { return filepath.Join(runDir, study.ProjectDir) }

// OraclePath is where records/oracle.json of an attempt lies.
func OraclePath(runDir string) string { return filepath.Join(recordsPath(runDir), study.OracleFile) }

// HeldOutPath is where records/heldout.json of an attempt lies.
func HeldOutPath(runDir string) string { return filepath.Join(recordsPath(runDir), study.HeldOutFile) }
