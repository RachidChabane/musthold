package accept

import (
	"path"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The directories of a task, relative to the task's own directory.
const (
	FixtureSubdir    = "fixture"
	ReferenceSubdir  = "reference"
	ViolationsSubdir = "violations"
	RefactorsSubdir  = "refactors"
	OracleSubdir     = "oracle"
)

// WithStandardPaths returns the task with its fixture and truth paths set to the
// standard layout under experiments/loop-1/tasks/<name>/.
func WithStandardPaths(task study.Task) study.Task {
	base := study.TaskDir(task.Name)
	task.Fixture = path.Join(base, FixtureSubdir)
	task.Truth = study.Truth{
		Reference:  path.Join(base, ReferenceSubdir),
		Violations: path.Join(base, ViolationsSubdir),
		Refactors:  path.Join(base, RefactorsSubdir),
		Oracle:     path.Join(base, OracleSubdir),
	}
	return task
}
