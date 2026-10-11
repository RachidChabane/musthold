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

// ErrGit is returned when a git command fails.
var ErrGit = errors.New("git failed")

var _ ports.SourceControl = Repository{}

// Repository is the git repository of a work tree.
type Repository struct {
	Process ports.ProcessRunner
	// Root is the work tree.
	Root string
}

func (r Repository) git(ctx context.Context, args ...string) (string, error) {
	result, err := r.Process.Run(ctx, ports.Command{Argv: append([]string{gitBinary}, args...), Dir: r.Root})
	switch {
	case err != nil:
		return "", fmt.Errorf("run git: %w", err)
	case result.ExitCode != 0:
		return "", fmt.Errorf("%w: git %s: %s", ErrGit, strings.Join(args, " "), strings.TrimSpace(string(result.Stderr)))
	}
	return string(result.Stdout), nil
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
