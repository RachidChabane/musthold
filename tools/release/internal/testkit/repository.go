// Package testkit builds throwaway git repositories for the tests of the release
// tool. It performs input and output, so only tests import it.
package testkit

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	fixedDate    = "1700000000 +0000"
	authorName   = "Release Fixture"
	authorEmail  = "fixture@example.com"
	directoryDir = 0o755
	nullDevice   = "/dev/null"
)

// Repository is a git repository in a temporary directory.
type Repository struct {
	Root string
	tb   testing.TB
}

// NewRepository initialises an empty repository whose commits carry a fixed
// author and date and are not signed, whatever the machine's git configuration.
func NewRepository(tb testing.TB) *Repository {
	tb.Helper()
	repository := &Repository{Root: tb.TempDir(), tb: tb}
	repository.Git("init", "--quiet", "--initial-branch=main")
	return repository
}

// Environment is the environment git and the scripts run with in a fixture: the
// machine's own, with the git configuration of the user and the system masked and
// an author and date fixed.
func Environment() []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+nullDevice, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME="+authorName, "GIT_AUTHOR_EMAIL="+authorEmail, "GIT_AUTHOR_DATE="+fixedDate,
		"GIT_COMMITTER_NAME="+authorName, "GIT_COMMITTER_EMAIL="+authorEmail, "GIT_COMMITTER_DATE="+fixedDate,
	)
}

// Git runs git in the repository and returns its trimmed output.
func (r *Repository) Git(arguments ...string) string {
	r.tb.Helper()
	output, code := r.Run("git", nil, arguments...)
	if code != 0 {
		r.tb.Fatalf("git %s: exit %d\n%s", strings.Join(arguments, " "), code, output)
	}
	return strings.TrimSpace(output)
}

// Run runs a command in the repository with the fixture environment plus extra
// variables, and returns its combined output and exit code.
func (r *Repository) Run(name string, extra []string, arguments ...string) (string, int) {
	r.tb.Helper()
	command := exec.Command(name, arguments...)
	command.Dir = r.Root
	command.Env = append(Environment(), extra...)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return output.String(), 0
	case errors.As(err, &exit):
		return output.String(), exit.ExitCode()
	}
	r.tb.Fatalf("run %s: %v", name, err)
	return "", -1
}

// Write creates a file, and its directory, with a mode.
func (r *Repository) Write(path, content string, mode fs.FileMode) {
	r.tb.Helper()
	target := filepath.Join(r.Root, path)
	if err := os.MkdirAll(filepath.Dir(target), directoryDir); err != nil {
		r.tb.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), mode); err != nil {
		r.tb.Fatal(err)
	}
	if err := os.Chmod(target, mode); err != nil {
		r.tb.Fatal(err)
	}
}

// CommitAll stages everything and commits it, returning the commit.
func (r *Repository) CommitAll(message string) string {
	r.tb.Helper()
	r.Git("add", "--all")
	r.Git("-c", "commit.gpgsign=false", "-c", "core.hooksPath="+nullDevice, "commit", "--quiet", "--message", message)
	return r.Git("rev-parse", "HEAD")
}

// Tag creates a lightweight tag at HEAD.
func (r *Repository) Tag(name string) { r.Git("tag", name) }

// CopyTree copies a directory tree, leaving out what skip returns true for.
func CopyTree(tb testing.TB, source, destination string, skip func(relative string, entry fs.DirEntry) bool) {
	tb.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative != "." && skip != nil && skip(relative, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, directoryDir)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		tb.Fatal(err)
	}
}
