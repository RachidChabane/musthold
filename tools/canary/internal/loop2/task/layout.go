// Package task holds the shapes the construction and acceptance of the loop-2
// study exchange (ADR 0017, "Tasks and oracle"): a task's description and its
// tier, the prompt it follows, the levers it declares, the size of its fixture,
// the provenance of its oracles and citations, its held-out violations and the
// layout of the study's directories. It is pure: it parses and formats bytes and
// reads nothing but the environment for the work directory.
package task

import (
	"os"
	"path"
	"path/filepath"

	l1accept "github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
)

// Locations in the repository, slash-separated from its root.
const (
	StudyDir    = "experiments/loop-2"
	ConfigFile  = StudyDir + "/loop-2.json"
	TasksDir    = StudyDir + "/tasks"
	DomainsFile = StudyDir + "/domains.json"
	DrawFile    = StudyDir + "/draw.json"
	// DockerClient is the docker command that names the study's engine, and
	// WithSlot the script that holds the machine-wide execution slots.
	DockerClient = StudyDir + "/script/docker"
	WithSlot     = "script/with-slot"
)

// The files of a task's directory beside loop-1's.
const (
	TaskFile           = "task.json"
	LeversFile         = "levers.json"
	CitationsFile      = "citations.json"
	ProvenanceFile     = "provenance.json"
	HeldOutFile        = "heldout.json"
	SecondOracleSubdir = "oracle-2"
)

// TaskDir is the directory of a task.
func TaskDir(name string) string { return TasksDir + "/" + name }

// TaskPath is a file of a task's directory.
func TaskPath(name, file string) string { return path.Join(TaskDir(name), file) }

// WorkEnv names the variable that holds the work directory, and DefaultWorkName
// the directory under the home directory used when it is unset; ProductEnv and
// DefaultProductName are the same for the product directory.
const (
	WorkEnv            = "LOOP2_WORK"
	DefaultWorkName    = ".musthold-loop-2"
	ProductEnv         = "LOOP2_PRODUCT"
	DefaultProductName = ".musthold-loop-2-product"
)

// DefaultWork is $LOOP2_WORK, or ~/.musthold-loop-2.
func DefaultWork() string { return fromEnvOrHome(WorkEnv, DefaultWorkName) }

// DefaultProduct is $LOOP2_PRODUCT, or ~/.musthold-loop-2-product.
func DefaultProduct() string { return fromEnvOrHome(ProductEnv, DefaultProductName) }

func fromEnvOrHome(variable, name string) string {
	if dir := os.Getenv(variable); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return name
	}
	return filepath.Join(home, name)
}

// WithStandardPaths returns the task with its fixture and truth paths set to the
// standard layout of its directory.
func WithStandardPaths(t Task) Task {
	base := TaskDir(t.Name)
	t.Fixture = path.Join(base, l1accept.FixtureSubdir)
	t.Truth = Truth{
		Reference:    path.Join(base, l1accept.ReferenceSubdir),
		Violations:   path.Join(base, l1accept.ViolationsSubdir),
		Refactors:    path.Join(base, l1accept.RefactorsSubdir),
		Oracle:       path.Join(base, l1accept.OracleSubdir),
		SecondOracle: path.Join(base, SecondOracleSubdir),
	}
	return t
}

// TruthPaths are the paths of a task's truth files that no fixture file may
// name.
func (t Task) TruthPaths() []string {
	base := TaskDir(t.Name)
	return []string{
		t.Truth.Reference, t.Truth.Violations, t.Truth.Refactors, t.Truth.Oracle, t.Truth.SecondOracle,
		path.Join(base, LeversFile), path.Join(base, CitationsFile), path.Join(base, ProvenanceFile),
	}
}
