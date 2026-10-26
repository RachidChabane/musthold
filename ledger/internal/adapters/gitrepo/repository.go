package gitrepo

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/ledger/internal/domain/ledger"
	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
)

const (
	branchRefPrefix = "refs/heads/"
	ledgerRef       = branchRefPrefix + format.LedgerBranch

	treeEntryFields = 3
	nul             = "\x00"
	emptyObject     = ""

	commitDateLayout = "%d +0000"
)

// Repository is the ledger of one git repository: the objects and the branch
// refs/heads/musthold/ledger.
type Repository struct {
	runner runner
}

// Option configures a Repository.
type Option func(*runner)

// WithEnvironment appends environment variables to every git command, for
// instance to isolate git from the user's configuration in tests.
func WithEnvironment(environment ...string) Option {
	return func(r *runner) { r.extraEnvironment = append(r.extraEnvironment, environment...) }
}

// New opens the repository in dir, which may be bare.
func New(dir string, options ...Option) *Repository {
	r := runner{dir: dir}
	for _, option := range options {
		option(&r)
	}
	return &Repository{runner: r}
}

// Tip returns the head of the ledger branch.
func (r *Repository) Tip() (ledger.ObjectID, bool, error) {
	res, err := r.runner.run(nil, nil, "rev-parse", "--verify", "--quiet", ledgerRef+"^{commit}")
	if err != nil {
		if res.exitCode == exitCodeNegative {
			return "", false, nil
		}
		return "", false, err
	}
	return ledger.ObjectID(strings.TrimSpace(string(res.stdout))), true, nil
}

// TreeOf returns the root tree of a commit.
func (r *Repository) TreeOf(commit ledger.ObjectID) (ledger.ObjectID, error) {
	out, err := r.runner.output(nil, "rev-parse", string(commit)+"^{tree}")
	if err != nil {
		return "", err
	}
	return ledger.ObjectID(strings.TrimSpace(string(out))), nil
}

// ReadTree lists the entries of a tree.
func (r *Repository) ReadTree(id ledger.ObjectID) ([]ledger.TreeEntry, error) {
	out, err := r.runner.output(nil, "ls-tree", "-z", string(id))
	if err != nil {
		return nil, err
	}
	return parseTreeEntries(out)
}

// ReadBlob returns the content of a blob.
func (r *Repository) ReadBlob(id ledger.ObjectID) ([]byte, error) {
	return r.runner.output(nil, "cat-file", "blob", string(id))
}

// WriteBlob stores content as a blob.
func (r *Repository) WriteBlob(content []byte) (ledger.ObjectID, error) {
	out, err := r.runner.output(content, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", err
	}
	return ledger.ObjectID(strings.TrimSpace(string(out))), nil
}

// WriteTree stores a tree of the given entries.
func (r *Repository) WriteTree(entries []ledger.TreeEntry) (ledger.ObjectID, error) {
	var input bytes.Buffer
	for _, entry := range entries {
		fmt.Fprintf(&input, "%s %s %s\t%s%s", entry.Mode, entry.Kind, entry.ID, entry.Name, nul)
	}
	out, err := r.runner.output(input.Bytes(), "mktree", "-z")
	if err != nil {
		return "", err
	}
	return ledger.ObjectID(strings.TrimSpace(string(out))), nil
}

// Commit writes a commit of tree on top of parent (none for a root commit),
// authored by the ledger identity at the given time.
func (r *Repository) Commit(tree ledger.ObjectID, parent *ledger.ObjectID, message string, at time.Time) (ledger.ObjectID, error) {
	args := []string{"commit-tree", string(tree), "-m", message}
	if parent != nil {
		args = append(args, "-p", string(*parent))
	}
	stamp := fmt.Sprintf(commitDateLayout, at.Unix())
	environment := []string{
		"GIT_AUTHOR_NAME=" + authorName, "GIT_AUTHOR_EMAIL=" + authorEmail, "GIT_AUTHOR_DATE=" + stamp,
		"GIT_COMMITTER_NAME=" + authorName, "GIT_COMMITTER_EMAIL=" + authorEmail, "GIT_COMMITTER_DATE=" + stamp,
	}
	res, err := r.runner.run(nil, environment, args...)
	if err != nil {
		return "", err
	}
	return ledger.ObjectID(strings.TrimSpace(string(res.stdout))), nil
}

// Advance moves the ledger branch from previous to next. The update names the
// value the branch must still have, so a concurrent writer makes it fail, and next
// must descend from previous: the branch only fast-forwards.
func (r *Repository) Advance(previous *ledger.ObjectID, next ledger.ObjectID) error {
	expected := emptyObject
	if previous != nil {
		expected = string(*previous)
		descends, err := r.runner.answer(exitCodeNegative, "merge-base", "--is-ancestor", expected, string(next))
		if err != nil {
			return err
		}
		if !descends {
			return refusal.Newf("%s is not a fast-forward of %s", next, expected)
		}
	}
	_, err := r.runner.output(nil, "update-ref", "-m", "ledger append", ledgerRef, string(next), expected)
	return err
}

// Files lists every file of a commit.
func (r *Repository) Files(commit ledger.ObjectID) ([]ledger.FileEntry, error) {
	out, err := r.runner.output(nil, "ls-tree", "-r", "-z", "--full-tree", string(commit))
	if err != nil {
		return nil, err
	}
	entries, err := parseTreeEntries(out)
	if err != nil {
		return nil, err
	}
	files := make([]ledger.FileEntry, 0, len(entries))
	for _, entry := range entries {
		files = append(files, ledger.FileEntry{Path: entry.Name, ID: entry.ID})
	}
	return files, nil
}

// parseTreeEntries reads the NUL-terminated output of `ls-tree -z`.
func parseTreeEntries(out []byte) ([]ledger.TreeEntry, error) {
	var entries []ledger.TreeEntry
	for _, record := range strings.Split(string(out), nul) {
		if record == "" {
			continue
		}
		header, name, found := strings.Cut(record, "\t")
		fields := strings.Fields(header)
		if !found || len(fields) != treeEntryFields {
			return nil, fmt.Errorf("unexpected ls-tree record %q", record)
		}
		entries = append(entries, ledger.TreeEntry{
			Mode: fields[0], Kind: ledger.EntryKind(fields[1]), ID: ledger.ObjectID(fields[2]), Name: name,
		})
	}
	return entries, nil
}

// History lists the commits reachable from the tip with their parents.
func (r *Repository) History(tip ledger.ObjectID) ([]ledger.CommitRecord, error) {
	out, err := r.runner.output(nil, "rev-list", "--parents", string(tip))
	if err != nil {
		return nil, err
	}
	var commits []ledger.CommitRecord
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		record := ledger.CommitRecord{ID: ledger.ObjectID(fields[0])}
		for _, parent := range fields[1:] {
			record.Parents = append(record.Parents, ledger.ObjectID(parent))
		}
		commits = append(commits, record)
	}
	return commits, nil
}

// Rewrites lists the paths some commit of the history deleted or modified, a path
// per line of `git log --diff-filter=DM`, merge commits included.
func (r *Repository) Rewrites(tip ledger.ObjectID) ([]string, error) {
	out, err := r.runner.output(nil, "-c", "core.quotepath=off", "log", "-z", "-m", "--no-renames",
		"--diff-filter=DM", "--name-only", "--format=", string(tip))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var paths []string
	for _, name := range strings.Split(string(out), nul) {
		name = strings.Trim(name, "\n")
		if name != "" && !seen[name] {
			seen[name] = true
			paths = append(paths, name)
		}
	}
	return paths, nil
}
