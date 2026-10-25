package scan

import "context"

// Issue is an issue a pull request closes, as the forge returned it. Title and
// body keep the exact bytes the forge sent.
type Issue struct {
	Number int
	Title  string
	Body   string
	URL    string
	// State is the forge's own word for the issue's state.
	State string
}

// PullRequest is a pull request as `scan <PR>` reads it. Title and body keep the
// exact bytes the forge sent, since they are saved and quoted byte for byte.
type PullRequest struct {
	Number int
	Title  string
	Body   string
	URL    string
	// BaseCommit is the tip of the branch the pull request targets.
	BaseCommit string
	// HeadCommit is the tip of the pull request's branch.
	HeadCommit string
	// ScanBase is the merge base of the local head and BaseCommit: the commit the
	// scan's change is measured from.
	ScanBase      string
	ClosingIssues []Issue
}

// PullRequestReader resolves a pull request number into what a scan needs. It
// refuses unless the work tree is at the pull request's head commit, and it never
// fetches or checks anything out.
type PullRequestReader interface {
	Read(ctx context.Context, number int) (PullRequest, error)
}
