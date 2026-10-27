package freeze

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Exec runs a host command and returns its standard output.
type Exec interface {
	Output(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

// ErrCommand is returned when a host command fails.
var ErrCommand = errors.New("command failed")

// OSExec runs commands on the host. The variables that redirect a repository
// command to another repository are removed from their environment.
type OSExec struct{}

var _ Exec = OSExec{}

var redirectingVariables = []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_COMMON_DIR"}

// Output implements Exec.
func (OSExec) Output(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = cleanEnvironment(os.Environ())
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s %s: %v: %s", ErrCommand, name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func cleanEnvironment(environment []string) []string {
	var kept []string
	for _, variable := range environment {
		name, _, _ := strings.Cut(variable, "=")
		redirect := false
		for _, forbidden := range redirectingVariables {
			redirect = redirect || name == forbidden
		}
		if !redirect {
			kept = append(kept, variable)
		}
	}
	return kept
}

// Repository reads the study's repository.
type Repository interface {
	// Head is the commit HEAD names.
	Head(ctx context.Context) (string, error)
	// PeelTag is the commit a tag names, or ErrNoTag.
	PeelTag(ctx context.Context, tag string) (string, error)
	// Parent is the first parent of a commit.
	Parent(ctx context.Context, rev string) (string, error)
	// IsAncestor reports whether ancestor is reachable from descendant.
	IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error)
	// Between lists, oldest first, the commits in (from, to] that touch dirs.
	Between(ctx context.Context, from, to string, dirs []string) ([]string, error)
	// Show is the content of a file at a revision.
	Show(ctx context.Context, rev, file string) ([]byte, error)
	// Changed lists the files a commit changes.
	Changed(ctx context.Context, rev string) ([]string, error)
	// TreeFiles lists the files of a revision under the given paths.
	TreeFiles(ctx context.Context, rev string, paths ...string) ([]string, error)
	// Tracked lists the files of the index under the given paths.
	Tracked(ctx context.Context, paths ...string) ([]string, error)
	// Modified lists the tracked files whose content differs from HEAD.
	Modified(ctx context.Context) ([]string, error)
}

// ErrNoTag is returned for a tag the repository does not hold.
var ErrNoTag = errors.New("no such tag")

// Git implements Repository with the git command line.
type Git struct {
	// Dir is the repository root.
	Dir  string
	Exec Exec
}

var _ Repository = Git{}

func (g Git) run(ctx context.Context, args ...string) ([]byte, error) {
	return g.Exec.Output(ctx, g.Dir, "git", args...)
}

func (g Git) lines(ctx context.Context, args ...string) ([]string, error) {
	output, err := g.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return splitLines(string(output), "\n"), nil
}

func splitLines(text, separator string) []string {
	var lines []string
	for _, line := range strings.Split(text, separator) {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func withPaths(args []string, paths []string) []string {
	if len(paths) == 0 {
		return args
	}
	return append(append(args, "--"), paths...)
}

// Head implements Repository.
func (g Git) Head(ctx context.Context) (string, error) { return g.revParse(ctx, "HEAD") }

// PeelTag implements Repository.
func (g Git) PeelTag(ctx context.Context, tag string) (string, error) {
	commit, err := g.revParse(ctx, "refs/tags/"+tag+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrNoTag, tag, err)
	}
	return commit, nil
}

// Parent implements Repository.
func (g Git) Parent(ctx context.Context, rev string) (string, error) {
	return g.revParse(ctx, rev+"^")
}

func (g Git) revParse(ctx context.Context, rev string) (string, error) {
	output, err := g.run(ctx, "rev-parse", "--verify", "--quiet", rev)
	return strings.TrimSpace(string(output)), err
}

// IsAncestor implements Repository.
func (g Git) IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	_, err := g.run(ctx, "merge-base", "--is-ancestor", ancestor, descendant)
	if err != nil && strings.Contains(err.Error(), "exit status 1") {
		return false, nil
	}
	return err == nil, err
}

// Between implements Repository.
func (g Git) Between(ctx context.Context, from, to string, dirs []string) ([]string, error) {
	return g.lines(ctx, withPaths([]string{"log", "--reverse", "--format=%H", from + ".." + to}, dirs)...)
}

// Show implements Repository.
func (g Git) Show(ctx context.Context, rev, file string) ([]byte, error) {
	return g.run(ctx, "show", rev+":"+file)
}

// Changed implements Repository.
func (g Git) Changed(ctx context.Context, rev string) ([]string, error) {
	return g.lines(ctx, "diff-tree", "--no-commit-id", "--name-only", "-r", "--root", rev)
}

// TreeFiles implements Repository.
func (g Git) TreeFiles(ctx context.Context, rev string, paths ...string) ([]string, error) {
	output, err := g.run(ctx, withPaths([]string{"ls-tree", "-r", "-z", "--name-only", rev}, paths)...)
	if err != nil {
		return nil, err
	}
	return splitLines(string(output), "\x00"), nil
}

// Tracked implements Repository.
func (g Git) Tracked(ctx context.Context, paths ...string) ([]string, error) {
	output, err := g.run(ctx, withPaths([]string{"ls-files", "-z"}, paths)...)
	if err != nil {
		return nil, err
	}
	return splitLines(string(output), "\x00"), nil
}

// Modified implements Repository.
func (g Git) Modified(ctx context.Context) ([]string, error) {
	output, err := g.run(ctx, "diff", "-z", "--name-only", "HEAD")
	if err != nil {
		return nil, err
	}
	return splitLines(string(output), "\x00"), nil
}
