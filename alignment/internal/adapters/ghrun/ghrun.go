// Package ghrun downloads the artifact of a CI run with the `gh` command line, so
// that the module itself opens no connection.
package ghrun

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
)

// gh command line.
const (
	ghBinary     = "gh"
	runCommand   = "run"
	downloadVerb = "download"
	nameFlag     = "--name"
	dirFlag      = "--dir"
	repoFlag     = "--repo"
)

// Errors of the adapter.
var (
	// ErrDownloadFailed is returned when gh exits with a failure.
	ErrDownloadFailed = errors.New("gh run download failed")
	// ErrNoRunner is returned when the adapter has no process runner.
	ErrNoRunner = errors.New("ghrun has no process runner")
)

// Client runs gh.
type Client struct {
	Process ports.ProcessRunner
	// Binary is the executable to run; empty stands for gh.
	Binary string
	// Dir is the directory gh runs in, which names the repository.
	Dir string
}

// Download fetches the named artifact of the run into dir. A repository, when
// given, is passed as owner/name.
func (c Client) Download(ctx context.Context, runID, artifact, dir, repository string) error {
	if c.Process == nil {
		return ErrNoRunner
	}
	binary := c.Binary
	if binary == "" {
		binary = ghBinary
	}
	argv := []string{binary, runCommand, downloadVerb, runID, nameFlag, artifact, dirFlag, dir}
	if repository != "" {
		argv = append(argv, repoFlag, repository)
	}
	result, err := c.Process.Run(ctx, ports.Command{Argv: argv, Dir: c.Dir})
	switch {
	case err != nil:
		return fmt.Errorf("run %s: %w", binary, err)
	case result.ExitCode != 0:
		return fmt.Errorf("%w: %s", ErrDownloadFailed, strings.TrimSpace(string(result.Stderr)))
	}
	return nil
}
