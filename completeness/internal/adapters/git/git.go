// Package git reads what the work tree of a project changed since a base commit,
// with the git command line: the files that differ from the base, including those
// the index does not know yet. It never writes to the repository, not even its
// index.
package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/completeness/internal/app/ports"
)

// Git command line.
const (
	binary            = "git"
	revParse          = "rev-parse"
	verifyFlag        = "--verify"
	quietFlag         = "--quiet"
	insideWorkTree    = "--is-inside-work-tree"
	mergeBase         = "merge-base"
	symbolicRef       = "symbolic-ref"
	shortFlag         = "--short"
	originHead        = "refs/remotes/origin/HEAD"
	configCommand     = "config"
	getFlag           = "--get"
	defaultBranchKey  = "init.defaultBranch"
	diffCommand       = "diff"
	nameStatusFlag    = "--name-status"
	nullTerminated    = "-z"
	findRenamesFlag   = "--find-renames"
	listFiles         = "ls-files"
	othersFlag        = "--others"
	excludeStandard   = "--exclude-standard"
	showCommand       = "show"
	headReference     = "HEAD"
	commitSuffix      = "^{commit}"
	trueOutput        = "true"
	noOptionalLocks   = "GIT_OPTIONAL_LOCKS=0"
	fieldTerminator   = "\x00"
	statusAdded       = 'A'
	statusDeleted     = 'D'
	statusRenamed     = 'R'
	statusCopied      = 'C'
	commandTimeout    = 60 * time.Second
	fallbackBranchOne = "main"
	fallbackBranchTwo = "master"
)

// ErrCommand is returned when git fails in a way that is not the absence of a
// repository or of a base.
var ErrCommand = errors.New("git failed")

var _ ports.ChangeSource = Source{}

// Source implements ports.ChangeSource over a project root.
type Source struct {
	Process ports.ProcessRunner
	Root    string
}

// Changes returns the files the work tree changed since the base. An explicit base
// reference is used as it is; an empty one stands for the merge base of HEAD and
// the default branch, which is `origin/HEAD`, then `init.defaultBranch`, then
// `main`, then `master`, and HEAD itself when none exists.
func (s Source) Changes(ctx context.Context, base string) (ports.ChangeSet, error) {
	if inside, err := s.run(ctx, revParse, insideWorkTree); err != nil || strings.TrimSpace(inside) != trueOutput {
		return ports.ChangeSet{}, ports.ErrNoRepository
	}
	commit, err := s.resolveBase(ctx, base)
	if err != nil {
		return ports.ChangeSet{}, err
	}
	changes, err := s.diff(ctx, commit)
	if err != nil {
		return ports.ChangeSet{}, err
	}
	untracked, err := s.untracked(ctx)
	if err != nil {
		return ports.ChangeSet{}, err
	}
	changes = append(changes, untracked...)
	return ports.ChangeSet{Base: commit, Changes: changes}, nil
}

func (s Source) run(ctx context.Context, args ...string) (string, error) {
	result, err := s.Process.Run(ctx, ports.Command{
		Argv: append([]string{binary}, args...), Dir: s.Root, Env: []string{noOptionalLocks}, Timeout: commandTimeout,
	})
	switch {
	case err != nil:
		return "", err
	case result.TimedOut:
		return "", fmt.Errorf("%w: git %s timed out", ErrCommand, args[0])
	case result.ExitCode != 0:
		return "", fmt.Errorf("%w: git %s exited with status %d: %s", ErrCommand, args[0], result.ExitCode, strings.TrimSpace(string(result.Stderr)))
	}
	return string(result.Stdout), nil
}

// commitOf resolves a reference to a commit id, empty when it names none.
func (s Source) commitOf(ctx context.Context, reference string) string {
	out, err := s.run(ctx, revParse, verifyFlag, quietFlag, reference+commitSuffix)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (s Source) resolveBase(ctx context.Context, base string) (string, error) {
	head := s.commitOf(ctx, headReference)
	if head == "" {
		return "", ports.ErrNoRepository
	}
	if base != "" {
		if commit := s.commitOf(ctx, base); commit != "" {
			return commit, nil
		}
		return "", fmt.Errorf("%w: %q names no commit", ports.ErrNoRepository, base)
	}
	for _, candidate := range s.defaultBranches(ctx) {
		if s.commitOf(ctx, candidate) == "" {
			continue
		}
		out, err := s.run(ctx, mergeBase, headReference, candidate)
		if err != nil {
			continue
		}
		return strings.TrimSpace(out), nil
	}
	return head, nil
}

// defaultBranches lists the names the default branch may have, most authoritative
// first.
func (s Source) defaultBranches(ctx context.Context) []string {
	var candidates []string
	if out, err := s.run(ctx, symbolicRef, quietFlag, shortFlag, originHead); err == nil {
		candidates = append(candidates, strings.TrimSpace(out))
	}
	if out, err := s.run(ctx, configCommand, getFlag, defaultBranchKey); err == nil {
		candidates = append(candidates, strings.TrimSpace(out))
	}
	return append(candidates, fallbackBranchOne, fallbackBranchTwo)
}

// diff lists the files whose work-tree content differs from the base commit.
func (s Source) diff(ctx context.Context, commit string) ([]ports.Change, error) {
	out, err := s.run(ctx, diffCommand, nameStatusFlag, nullTerminated, findRenamesFlag, commit)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(out, fieldTerminator)
	var changes []ports.Change
	for index := 0; index < len(fields) && fields[index] != ""; {
		status := fields[index][0]
		index++
		oldPath := fields[index]
		path := oldPath
		index++
		if status == statusRenamed || status == statusCopied {
			path = fields[index]
			index++
		}
		change, err := s.change(ctx, commit, status, oldPath, path)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func (s Source) change(ctx context.Context, commit string, status byte, oldPath, path string) (ports.Change, error) {
	change := ports.Change{Path: path, OldPath: oldPath}
	if status != statusAdded {
		before, err := s.run(ctx, showCommand, commit+":"+oldPath)
		if err != nil {
			return ports.Change{}, err
		}
		change.Before = []byte(before)
	}
	if status != statusDeleted {
		after, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(path)))
		if err != nil {
			return ports.Change{}, fmt.Errorf("read %s: %w", path, err)
		}
		change.After = after
	}
	return change, nil
}

// untracked lists the files the repository does not track and does not ignore,
// each as an added file.
func (s Source) untracked(ctx context.Context) ([]ports.Change, error) {
	out, err := s.run(ctx, listFiles, othersFlag, excludeStandard, nullTerminated)
	if err != nil {
		return nil, err
	}
	var changes []ports.Change
	for _, path := range strings.Split(out, fieldTerminator) {
		if path == "" {
			continue
		}
		after, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(path)))
		if err != nil {
			continue
		}
		changes = append(changes, ports.Change{Path: path, OldPath: path, After: after})
	}
	return changes, nil
}
