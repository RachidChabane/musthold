// Package gitsource reads the version-control state a scan names with the git
// command line: the head commit, a base revision, the tracked files, whether a
// tracked file changed, the lines a change touched and the repository's name.
// It never writes to the repository.
package gitsource

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
)

// Git command line.
const (
	gitBinary          = "git"
	revParse           = "rev-parse"
	verifyFlag         = "--verify"
	commitSuffix       = "^{commit}"
	headRevision       = "HEAD"
	statusCommand      = "status"
	porcelainFlag      = "--porcelain"
	trackedOnlyFlag    = "--untracked-files=no"
	listFiles          = "ls-files"
	nulFlag            = "-z"
	diffCommand        = "diff"
	zeroContextFlag    = "--unified=0"
	noColorFlag        = "--no-color"
	noExternalDiffFlag = "--no-ext-diff"
	renamesFlag        = "-M"
	mergeBaseCommand   = "merge-base"
	catFileCommand     = "cat-file"
	existsFlag         = "-e"
	blobType           = "blob"
	lsTreeCommand      = "ls-tree"
	blobEntry          = " blob "
	entrySeparator     = "\t"
	remoteCommand      = "remote"
	getURL             = "get-url"
	originRemote       = "origin"
	pathspecSeparator  = "--"
	allPaths           = "."
	// excludedOutput keeps scan reports out of the dirty check.
	excludedOutput = ":(exclude).spec-proof"
	// Markers of `git diff` output.
	newFilePrefix = "+++ "
	newFileSide   = "b/"
	hunkPrefix    = "@@ "
	hunkNewSide   = "+"
	countSplit    = ","
	devNull       = "/dev/null"
	// gitSuffix ends the URL of a remote repository.
	gitSuffix      = ".git"
	pathSeparator  = "/"
	scpSeparator   = ":"
	namePartsCount = 2
)

// Errors of the adapter.
var (
	// ErrGit is returned when a git command fails.
	ErrGit = errors.New("git failed")
	// ErrNoMergeBase is returned when two commits share no ancestor.
	ErrNoMergeBase = errors.New("the commits share no ancestor")
	// ErrNotInTree is returned when a path names no file of the head commit.
	ErrNotInTree = errors.New("file is not in the head commit")
)

var _ ports.SourceControl = Repository{}

// Repository is the git repository of a work tree.
type Repository struct {
	Process ports.ProcessRunner
	// Root is the work tree.
	Root string
}

func (r Repository) run(ctx context.Context, args ...string) (ports.CommandResult, error) {
	result, err := r.Process.Run(ctx, ports.Command{Argv: append([]string{gitBinary}, args...), Dir: r.Root})
	if err != nil {
		return result, fmt.Errorf("run git: %w", err)
	}
	return result, nil
}

func (r Repository) git(ctx context.Context, args ...string) (string, error) {
	result, err := r.run(ctx, args...)
	switch {
	case err != nil:
		return "", err
	case result.ExitCode != 0:
		return "", failure(args, result)
	}
	return string(result.Stdout), nil
}

func failure(args []string, result ports.CommandResult) error {
	return fmt.Errorf("%w: git %s: %s", ErrGit, strings.Join(args, " "), strings.TrimSpace(string(result.Stderr)))
}

// MergeBase is the best common ancestor of the head commit and a revision, as a
// full commit id. found is false when the revision names no commit of the local
// object database; the repository is never fetched into.
func (r Repository) MergeBase(ctx context.Context, revision string) (base string, found bool, err error) {
	probe := []string{catFileCommand, existsFlag, revision + commitSuffix}
	result, err := r.run(ctx, probe...)
	switch {
	case err != nil:
		return "", false, err
	case result.ExitCode != 0:
		return "", false, nil
	}
	args := []string{mergeBaseCommand, headRevision, revision + commitSuffix}
	result, err = r.run(ctx, args...)
	switch {
	case err != nil:
		return "", true, err
	case result.ExitCode != 0 && strings.TrimSpace(string(result.Stderr)) == "":
		return "", true, fmt.Errorf("%w: %s", ErrNoMergeBase, revision)
	case result.ExitCode != 0:
		return "", true, failure(args, result)
	}
	return strings.TrimSpace(string(result.Stdout)), true, nil
}

// ReadAtHead reads a file, relative POSIX, from the tree of the head commit,
// not from the work tree. A path that is not a file of that tree gives
// ErrNotInTree.
func (r Repository) ReadAtHead(ctx context.Context, path string) ([]byte, error) {
	listing, err := r.git(ctx, lsTreeCommand, nulFlag, headRevision, pathspecSeparator, path)
	if err != nil {
		return nil, err
	}
	description, name, _ := strings.Cut(strings.TrimSuffix(listing, "\x00"), entrySeparator)
	if name != path || !strings.Contains(description, blobEntry) {
		return nil, fmt.Errorf("%w: %s", ErrNotInTree, path)
	}
	content, err := r.git(ctx, catFileCommand, blobType, headRevision+scpSeparator+path)
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

// Head is the commit the work tree is at.
func (r Repository) Head(ctx context.Context) (string, error) {
	return r.Resolve(ctx, headRevision)
}

// Resolve gives the full id of the commit a revision names.
func (r Repository) Resolve(ctx context.Context, revision string) (string, error) {
	output, err := r.git(ctx, revParse, verifyFlag, revision+commitSuffix)
	return strings.TrimSpace(output), err
}

// Dirty reports whether a tracked file differs from the head commit; scan
// reports are not looked at.
func (r Repository) Dirty(ctx context.Context) (bool, error) {
	output, err := r.git(ctx, statusCommand, porcelainFlag, trackedOnlyFlag, pathspecSeparator, allPaths, excludedOutput)
	return strings.TrimSpace(output) != "", err
}

// TrackedFiles lists the files of the index, relative POSIX.
func (r Repository) TrackedFiles(ctx context.Context) ([]string, error) {
	output, err := r.git(ctx, listFiles, nulFlag)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, file := range strings.Split(output, "\x00") {
		if file != "" {
			files = append(files, file)
		}
	}
	return files, nil
}

// ChangedLines lists, per file of the head commit, the lines that differ from
// the base. A pure deletion marks the line it follows, so the function it was
// cut from counts as changed.
func (r Repository) ChangedLines(ctx context.Context, base string) (map[string][]int, error) {
	output, err := r.git(ctx, diffCommand, zeroContextFlag, noColorFlag, noExternalDiffFlag, renamesFlag, base, headRevision)
	if err != nil {
		return nil, err
	}
	return ParseChangedLines(output), nil
}

// ParseChangedLines reads the new-side lines of a zero-context unified diff.
func ParseChangedLines(diff string) map[string][]int {
	changed := map[string][]int{}
	file := ""
	scanner := bufio.NewScanner(strings.NewReader(diff))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, newFilePrefix):
			file = strings.TrimPrefix(strings.TrimPrefix(line, newFilePrefix), newFileSide)
			if file == devNull {
				file = ""
			}
		case strings.HasPrefix(line, hunkPrefix) && file != "":
			first, count, ok := newSide(line)
			if !ok {
				continue
			}
			if count == 0 {
				changed[file] = append(changed[file], max(first, 1))
			}
			for offset := range count {
				changed[file] = append(changed[file], first+offset)
			}
		}
	}
	return changed
}

// newSide reads `+start,count` of a hunk header; an omitted count is one.
func newSide(header string) (first, count int, ok bool) {
	for _, field := range strings.Fields(header) {
		if !strings.HasPrefix(field, hunkNewSide) {
			continue
		}
		start, length, hasCount := strings.Cut(strings.TrimPrefix(field, hunkNewSide), countSplit)
		first, err := strconv.Atoi(start)
		if err != nil {
			return 0, 0, false
		}
		count = 1
		if hasCount {
			if count, err = strconv.Atoi(length); err != nil {
				return 0, 0, false
			}
		}
		return first, count, true
	}
	return 0, 0, false
}

// Name is `owner/name` of the origin remote, or the name of the work tree's
// directory.
func (r Repository) Name(ctx context.Context) string {
	if output, err := r.git(ctx, remoteCommand, getURL, originRemote); err == nil {
		if name, ok := NameOfRemote(strings.TrimSpace(output)); ok {
			return name
		}
	}
	return filepath.Base(r.Root)
}

// NameOfRemote reads `owner/name` from the URL of a remote, in its URL or its
// scp-like form.
func NameOfRemote(url string) (string, bool) {
	trimmed := strings.TrimSuffix(strings.TrimSuffix(url, pathSeparator), gitSuffix)
	if index := strings.LastIndex(trimmed, scpSeparator); index >= 0 && !strings.Contains(trimmed[index:], pathSeparator+pathSeparator) {
		trimmed = strings.ReplaceAll(trimmed[:index], scpSeparator, pathSeparator) + pathSeparator + trimmed[index+1:]
	}
	parts := strings.Split(trimmed, pathSeparator)
	if len(parts) < namePartsCount || parts[len(parts)-1] == "" || parts[len(parts)-2] == "" {
		return "", false
	}
	return parts[len(parts)-2] + pathSeparator + parts[len(parts)-1], true
}
