// Package ghpr reads a pull request and the issues it closes with the `gh`
// command line, so that the module itself opens no connection. The diff is not
// read: it is never a quotable source. The work tree must already be at the pull
// request's head commit; the adapter never fetches and never checks out.
package ghpr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
)

// gh command line.
const (
	ghBinary      = "gh"
	prCommand     = "pr"
	issueCommand  = "issue"
	viewVerb      = "view"
	jsonFlag      = "--json"
	repoFlag      = "--repo"
	repoSeparator = "/"
	// prFields and issueFields are the fields read from gh.
	prFields    = "number,title,body,url,baseRefOid,headRefOid,closingIssuesReferences"
	issueFields = "number,title,body,url,state"
	// promptsOff keeps gh from waiting for a keyboard.
	promptsOff = "GH_PROMPT_DISABLED=1"
	// authCommand and notLoggedInMsg recognise gh's complaint about a missing sign-in.
	authCommand    = "gh auth login"
	notLoggedInMsg = "not logged in"
	// checkoutCommand and fetchCommand are what the refusals tell the user to run.
	checkoutCommand = "gh pr checkout"
	fetchCommand    = "git fetch"
)

// Errors of the adapter.
var (
	// ErrNoRunner is returned when the adapter has no process runner.
	ErrNoRunner = errors.New("ghpr has no process runner")
	// ErrNoRepository is returned when the adapter has no local repository.
	ErrNoRepository = errors.New("ghpr has no local repository")
	// ErrGHUnavailable is returned when gh cannot be run.
	ErrGHUnavailable = errors.New("gh cannot be run; install the GitHub CLI (https://cli.github.com)")
	// ErrNotAuthenticated is returned when gh is not signed in.
	ErrNotAuthenticated = errors.New("gh is not authenticated; run `" + authCommand + "`")
	// ErrViewFailed is returned when gh exits with a failure.
	ErrViewFailed = errors.New("gh view failed")
	// ErrMalformedOutput is returned when gh prints JSON that is not what was asked for.
	ErrMalformedOutput = errors.New("gh printed an unreadable answer")
	// ErrHeadMismatch is returned when the work tree is not at the pull request's head.
	ErrHeadMismatch = errors.New("the work tree is not at the head commit of the pull request")
	// ErrBaseMissing is returned when the pull request's base commit is not in the
	// local object database.
	ErrBaseMissing = errors.New("the base commit of the pull request is not in the local repository")
)

// HeadMismatchError says which commit the work tree is at and which it should be
// at. It matches ErrHeadMismatch.
type HeadMismatchError struct {
	Number      int
	LocalHead   string
	PullRequest string
}

func (e HeadMismatchError) Error() string {
	return fmt.Sprintf("%v: HEAD is %s, pull request #%d is at %s; run `%s %d` first",
		ErrHeadMismatch, e.LocalHead, e.Number, e.PullRequest, checkoutCommand, e.Number)
}

// Is matches ErrHeadMismatch.
func (e HeadMismatchError) Is(target error) bool { return target == ErrHeadMismatch }

// BaseMissingError names the commit to fetch. It matches ErrBaseMissing.
type BaseMissingError struct {
	Number     int
	BaseCommit string
}

func (e BaseMissingError) Error() string {
	return fmt.Sprintf("%v: commit %s (base of pull request #%d); run `%s` so that it is available, then scan again",
		ErrBaseMissing, e.BaseCommit, e.Number, fetchCommand)
}

// Is matches ErrBaseMissing.
func (e BaseMissingError) Is(target error) bool { return target == ErrBaseMissing }

// Repository is the local git state the adapter needs: the commit the work tree
// is at and its merge base with another commit.
type Repository interface {
	Head(ctx context.Context) (string, error)
	// MergeBase gives the merge base of the head and a revision; found is false
	// when the revision is not a local commit.
	MergeBase(ctx context.Context, revision string) (base string, found bool, err error)
}

// Client reads pull requests through gh.
type Client struct {
	Process ports.ProcessRunner
	// Repository is the local repository the work tree belongs to.
	Repository Repository
	// Binary is the executable to run; empty stands for gh.
	Binary string
	// Dir is the work tree gh runs in, which names the repository.
	Dir string
}

var _ scan.PullRequestReader = Client{}

type repositoryRef struct {
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
}

func (r repositoryRef) qualified() string {
	if r.Name == "" || r.Owner.Login == "" {
		return ""
	}
	return r.Owner.Login + repoSeparator + r.Name
}

type issueDocument struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
	State  string `json:"state"`
}

type closingReference struct {
	Number     int           `json:"number"`
	Repository repositoryRef `json:"repository"`
}

type prDocument struct {
	Number     int                `json:"number"`
	Title      string             `json:"title"`
	Body       string             `json:"body"`
	URL        string             `json:"url"`
	BaseRefOid string             `json:"baseRefOid"`
	HeadRefOid string             `json:"headRefOid"`
	Closing    []closingReference `json:"closingIssuesReferences"`
}

// Read resolves the pull request, checks that the work tree is at its head and
// finds the scan base, then reads each issue the pull request closes.
func (c Client) Read(ctx context.Context, number int) (scan.PullRequest, error) {
	if c.Process == nil {
		return scan.PullRequest{}, ErrNoRunner
	}
	if c.Repository == nil {
		return scan.PullRequest{}, ErrNoRepository
	}
	var document prDocument
	if err := c.view(ctx, &document, prCommand, viewVerb, strconv.Itoa(number), jsonFlag, prFields); err != nil {
		return scan.PullRequest{}, fmt.Errorf("pull request #%d: %w", number, err)
	}
	scanBase, err := c.checkLocalState(ctx, number, document)
	if err != nil {
		return scan.PullRequest{}, err
	}
	issues, err := c.closingIssues(ctx, document.Closing)
	if err != nil {
		return scan.PullRequest{}, err
	}
	return scan.PullRequest{
		Number: document.Number, Title: document.Title, Body: document.Body, URL: document.URL,
		BaseCommit: document.BaseRefOid, HeadCommit: document.HeadRefOid, ScanBase: scanBase,
		ClosingIssues: issues,
	}, nil
}

func (c Client) checkLocalState(ctx context.Context, number int, document prDocument) (string, error) {
	head, err := c.Repository.Head(ctx)
	if err != nil {
		return "", fmt.Errorf("local head: %w", err)
	}
	if head != document.HeadRefOid {
		return "", HeadMismatchError{Number: number, LocalHead: head, PullRequest: document.HeadRefOid}
	}
	base, found, err := c.Repository.MergeBase(ctx, document.BaseRefOid)
	switch {
	case err != nil:
		return "", fmt.Errorf("merge base with %s: %w", document.BaseRefOid, err)
	case !found:
		return "", BaseMissingError{Number: number, BaseCommit: document.BaseRefOid}
	}
	return base, nil
}

func (c Client) closingIssues(ctx context.Context, references []closingReference) ([]scan.Issue, error) {
	issues := make([]scan.Issue, 0, len(references))
	for _, reference := range references {
		args := []string{issueCommand, viewVerb, strconv.Itoa(reference.Number), jsonFlag, issueFields}
		if repository := reference.Repository.qualified(); repository != "" {
			args = append(args, repoFlag, repository)
		}
		var document issueDocument
		if err := c.view(ctx, &document, args...); err != nil {
			return nil, fmt.Errorf("issue #%d: %w", reference.Number, err)
		}
		issues = append(issues, scan.Issue{
			Number: document.Number, Title: document.Title, Body: document.Body,
			URL: document.URL, State: document.State,
		})
	}
	return issues, nil
}

// view runs gh and decodes its JSON answer into target.
func (c Client) view(ctx context.Context, target any, args ...string) error {
	binary := c.Binary
	if binary == "" {
		binary = ghBinary
	}
	result, err := c.Process.Run(ctx, ports.Command{
		Argv: append([]string{binary}, args...), Dir: c.Dir, Env: []string{promptsOff},
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGHUnavailable, err)
	}
	if result.ExitCode != 0 {
		return failure(result)
	}
	decoder := json.NewDecoder(bytes.NewReader(result.Stdout))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedOutput, err)
	}
	return nil
}

func failure(result ports.CommandResult) error {
	stderr := strings.TrimSpace(string(result.Stderr))
	if strings.Contains(stderr, authCommand) || strings.Contains(strings.ToLower(stderr), notLoggedInMsg) {
		return fmt.Errorf("%w: %s", ErrNotAuthenticated, stderr)
	}
	return fmt.Errorf("%w: %s", ErrViewFailed, stderr)
}
