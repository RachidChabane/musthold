package adapters

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
)

const (
	gitBinary         = "git"
	gitLocaleOverride = "LC_ALL=C"
	statusPathOffset  = 3
	commitFieldCount  = 2
	fieldTab          = "\t"
)

// Git reads a repository by running git in its root.
type Git struct{ runner commandRunner }

// NewGit returns a reader of the repository rooted at dir.
func NewGit(dir string) Git { return Git{runner: commandRunner{dir: dir}} }

func (g Git) output(arguments ...string) (string, error) {
	output, err := g.runner.run([]string{gitLocaleOverride}, gitBinary, arguments...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(arguments, " "), err, strings.TrimSpace(output))
	}
	return strings.TrimSpace(output), nil
}

// Head returns the commit HEAD names.
func (g Git) Head() (string, error) { return g.output("rev-parse", "HEAD") }

// TagCommit returns the commit a tag names, and false when the tag does not exist.
func (g Git) TagCommit(tag string) (string, bool, error) {
	output, err := g.runner.run([]string{gitLocaleOverride}, gitBinary, "rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}")
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(output), true, nil
}

// DirtyPaths lists the paths the working tree changed or added, ignored files aside.
func (g Git) DirtyPaths() ([]string, error) {
	output, err := g.runner.run([]string{gitLocaleOverride}, gitBinary, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("git status: %w\n%s", err, strings.TrimSpace(output))
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		if len(line) > statusPathOffset {
			paths = append(paths, line[statusPathOffset:])
		}
	}
	return paths, nil
}

// Tags lists the tags of the repository.
func (g Git) Tags() ([]string, error) {
	output, err := g.output("tag", "--list")
	if err != nil || output == "" {
		return nil, err
	}
	return strings.Split(output, "\n"), nil
}

// CommitTime returns the committer time of a commit.
func (g Git) CommitTime(commit string) (time.Time, error) {
	output, err := g.output("show", "-s", "--format=%ct", commit)
	if err != nil {
		return time.Time{}, err
	}
	seconds, err := strconv.ParseInt(output, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("commit time %q: %w", output, err)
	}
	return domain.CommitTime(seconds), nil
}

// PreviousTag returns the highest tag of the module below the given tag.
func (g Git) PreviousTag(module string, before domain.Tag) (string, error) {
	output, err := g.output("tag", "--list", module+"/v*")
	if err != nil || output == "" {
		return "", err
	}
	var best *domain.Tag
	for _, name := range strings.Split(output, "\n") {
		candidate, err := domain.ParseTag(name)
		if err != nil || candidate.Version.Compare(before.Version) >= 0 {
			continue
		}
		if best == nil || candidate.Version.Compare(best.Version) > 0 {
			best = &candidate
		}
	}
	if best == nil {
		return "", nil
	}
	return best.String(), nil
}

// CommitsSince lists the commits after a tag, up to HEAD, that touched any of the
// paths; with no tag it lists the whole history.
func (g Git) CommitsSince(previous string, paths []string) ([]domain.Commit, error) {
	arguments := []string{"log", "--format=%h" + fieldTab + "%s"}
	if previous != "" {
		arguments = append(arguments, previous+"..HEAD")
	}
	arguments = append(arguments, "--")
	arguments = append(arguments, paths...)
	output, err := g.output(arguments...)
	if err != nil || output == "" {
		return nil, err
	}
	var commits []domain.Commit
	for _, line := range strings.Split(output, "\n") {
		fields := strings.SplitN(line, fieldTab, commitFieldCount)
		if len(fields) == commitFieldCount {
			commits = append(commits, domain.Commit{Hash: fields[0], Subject: fields[1]})
		}
	}
	return commits, nil
}
