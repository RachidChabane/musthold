// Package hostgit publishes a host bundle under a git ref with git plumbing: the
// files become blobs, a tree and a commit written to the repository's object
// store, and the ref points at the commit. The working tree, the index and the
// current branch are never touched.
package hostgit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/musthold/completeness/internal/app/ports"
	"github.com/RachidChabane/musthold/format/hostproto/hostbundle"
)

// Git command line.
const (
	gitBinary        = "git"
	envIndexFile     = "GIT_INDEX_FILE="
	indexFileName    = "index"
	blobMode         = "100644"
	commitIdentity   = "musthold"
	commitEmail      = "musthold@localhost"
	commitMessage    = "musthold host bundle"
	scratchPrefix    = "musthold-hostgit-"
	fileMode         = 0o644
	forcePrefix      = "+"
	refspecSeparator = ":"
)

// Errors of the adapter.
var (
	// ErrGit is returned when a git command fails.
	ErrGit = errors.New("git failed")
	// ErrNoRunner is returned when the adapter has no process runner.
	ErrNoRunner = errors.New("hostgit has no process runner")
)

// Repository is the git repository of a project.
type Repository struct {
	Process ports.ProcessRunner
	// Root is the working directory of the repository.
	Root string
}

func (r Repository) git(ctx context.Context, env []string, args ...string) (string, error) {
	if r.Process == nil {
		return "", ErrNoRunner
	}
	result, err := r.Process.Run(ctx, ports.Command{Argv: append([]string{gitBinary}, args...), Dir: r.Root, Env: env})
	switch {
	case err != nil:
		return "", fmt.Errorf("run git: %w", err)
	case result.ExitCode != 0:
		return "", fmt.Errorf("%w: git %s: %s", ErrGit, strings.Join(args, " "), strings.TrimSpace(string(result.Stderr)))
	}
	return strings.TrimSpace(string(result.Stdout)), nil
}

// HeadSHA is the commit the working tree is at.
func (r Repository) HeadSHA(ctx context.Context) (string, error) {
	return r.git(ctx, nil, "rev-parse", "HEAD")
}

// HasUncommittedChanges reports whether a tracked file differs from the head
// commit, which makes the requests the host answered differ from the ones CI
// recomputes from that commit.
func (r Repository) HasUncommittedChanges(ctx context.Context) (bool, error) {
	status, err := r.git(ctx, nil, "status", "--porcelain", "--untracked-files=no")
	return status != "", err
}

// Publish writes the files as a tree and a commit and points ref at the commit.
// It returns the commit's id. A ref that exists is replaced.
func (r Repository) Publish(ctx context.Context, ref string, files []hostbundle.File) (string, error) {
	scratch, err := os.MkdirTemp("", scratchPrefix)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	index := []string{envIndexFile + filepath.Join(scratch, indexFileName)}
	for _, file := range files {
		blob, err := r.writeBlob(ctx, scratch, file)
		if err != nil {
			return "", err
		}
		if _, err := r.git(ctx, index, "update-index", "--add", "--cacheinfo", blobMode+","+blob+","+file.Path); err != nil {
			return "", err
		}
	}
	tree, err := r.git(ctx, index, "write-tree")
	if err != nil {
		return "", err
	}
	commit, err := r.git(ctx, nil, "-c", "user.name="+commitIdentity, "-c", "user.email="+commitEmail, "commit-tree", tree, "-m", commitMessage)
	if err != nil {
		return "", err
	}
	if _, err := r.git(ctx, nil, "update-ref", ref, commit); err != nil {
		return "", err
	}
	return commit, nil
}

func (r Repository) writeBlob(ctx context.Context, scratch string, file hostbundle.File) (string, error) {
	path := filepath.Join(scratch, "blob")
	if err := os.WriteFile(path, file.Content, fileMode); err != nil {
		return "", err
	}
	return r.git(ctx, nil, "hash-object", "-w", path)
}

// PushRef publishes the ref to a remote, replacing the remote's copy.
func (r Repository) PushRef(ctx context.Context, remote, ref string) error {
	_, err := r.git(ctx, nil, "push", remote, forcePrefix+ref+refspecSeparator+ref)
	return err
}
