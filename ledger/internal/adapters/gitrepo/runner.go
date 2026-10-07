// Package gitrepo is the adapter that keeps the ledger in a git repository. It uses
// git plumbing only (hash-object, mktree, commit-tree, update-ref, ls-tree,
// cat-file, rev-list, log, fetch and push): no checkout, no index, no working
// tree, so a bare repository serves as well as a clone.
package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	gitBinary = "git"

	// exitCodeNegative is the exit status of `rev-parse --verify --quiet` and of
	// `merge-base --is-ancestor` for a negative answer.
	exitCodeNegative = 1
	// exitCodeNoMatchingRemoteRef is the exit status of `ls-remote --exit-code`
	// when the remote has no matching ref.
	exitCodeNoMatchingRemoteRef = 2

	authorName  = "spec-proof ledger"
	authorEmail = "ledger@spec-proof.invalid"
)

// runner runs git in one repository.
type runner struct {
	dir string
	// extraEnvironment is appended to the environment of every command.
	extraEnvironment []string
}

// result of a git command that exited normally or not.
type result struct {
	stdout   []byte
	exitCode int
}

// run runs git with the given standard input and arguments. A non-zero exit
// status is returned as a result with its code; only a failure to run git at all
// is an error.
func (r runner) run(stdin []byte, environment []string, args ...string) (result, error) {
	command := exec.Command(gitBinary, append([]string{"-C", r.dir}, args...)...)
	command.Env = append(append(os.Environ(), r.extraEnvironment...), environment...)
	if stdin != nil {
		command.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	var exitError *exec.ExitError
	switch {
	case err == nil:
		return result{stdout: stdout.Bytes()}, nil
	case errors.As(err, &exitError):
		return result{stdout: stdout.Bytes(), exitCode: exitError.ExitCode()}, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	default:
		return result{}, fmt.Errorf("run git: %w", err)
	}
}

// output runs git and returns its standard output, failing on any non-zero status.
func (r runner) output(stdin []byte, args ...string) ([]byte, error) {
	res, err := r.run(stdin, nil, args...)
	if err != nil {
		return nil, err
	}
	return res.stdout, nil
}

// answer runs a git command that answers yes with status 0 and no with the given
// status, and fails on any other status.
func (r runner) answer(negative int, args ...string) (bool, error) {
	res, err := r.run(nil, nil, args...)
	if err == nil {
		return true, nil
	}
	if res.exitCode == negative {
		return false, nil
	}
	return false, err
}
