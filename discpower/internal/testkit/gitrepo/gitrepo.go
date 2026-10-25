// Package gitrepo builds throwaway git repositories for tests. The environment
// variables that bind git to an enclosing repository are cleared for the test, so
// that no command of a test can touch the repository the test runs in.
package gitrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	dirMode  = 0o755
	fileMode = 0o644
	// localEnvVars is the variables `git rev-parse --local-env-vars` lists.
	localEnvVars = "GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CONFIG GIT_CONFIG_PARAMETERS GIT_CONFIG_COUNT " +
		"GIT_OBJECT_DIRECTORY GIT_DIR GIT_WORK_TREE GIT_IMPLICIT_WORK_TREE GIT_GRAFT_FILE GIT_INDEX_FILE " +
		"GIT_NO_REPLACE_OBJECTS GIT_REPLACE_REF_BASE GIT_PREFIX GIT_SHALLOW_FILE GIT_COMMON_DIR"
)

// Repo is a temporary repository.
type Repo struct {
	Dir string
	t   *testing.T
}

// Isolate unsets, for the test, every variable that binds git to a repository.
func Isolate(t *testing.T) {
	t.Helper()
	for _, name := range strings.Fields(localEnvVars) {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

// New initialises an empty repository in a temporary directory.
func New(t *testing.T) *Repo {
	t.Helper()
	Isolate(t)
	repo := &Repo{Dir: t.TempDir(), t: t}
	repo.Git("init", "-q", "-b", "main")
	return repo
}

// Git runs git in the repository and returns its trimmed output.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	config := []string{"-c", "user.name=scan", "-c", "user.email=scan@example.com", "-c", "commit.gpgsign=false"}
	command := exec.Command("git", append(config, args...)...)
	command.Dir = r.Dir
	output, err := command.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

// Write creates or replaces a file of the work tree.
func (r *Repo) Write(file, content string) {
	r.t.Helper()
	path := filepath.Join(r.Dir, file)
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), fileMode); err != nil {
		r.t.Fatal(err)
	}
}

// Commit stages everything and commits it, returning the new commit id.
func (r *Repo) Commit(message string) string {
	r.t.Helper()
	r.Git("add", "-A")
	r.Git("commit", "-qm", message)
	return r.Git("rev-parse", "HEAD")
}
