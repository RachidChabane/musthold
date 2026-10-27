package detect

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
)

//go:embed helper.py
var helperSource string

// Bounds and environment of the extraction command.
const (
	extractLimit = 120 * time.Second
	pythonBinary = "python3"
	// isolatedFlag keeps the interpreter from loading code of the working directory.
	isolatedFlag = "-I"
	commandFlag  = "-c"
	// stderrExcerpt is how much of a failed command's error output an error keeps.
	stderrExcerpt = 400
)

// containerEnv is the environment of the extraction command: the interpreter and
// git, and no home directory or system configuration.
var containerEnv = []string{
	"PATH=/usr/local/bin:/usr/bin:/bin",
	"HOME=/nonexistent",
	"GIT_CONFIG_NOSYSTEM=1",
	"PYTHONDONTWRITEBYTECODE=1",
}

// Errors of the extraction.
var (
	ErrExtractFailed = errors.New("the syntax-tree extraction failed")
	ErrExtractOutput = errors.New("the syntax-tree extraction printed no readable facts")
)

// Extractor reads the facts of a project at its equipped commit and in its working
// tree.
type Extractor interface {
	Extract(ctx context.Context, projectDir, equippedCommit string) (Snapshot, error)
}

// BoxExtractor runs the embedded Python helper in the study's container, so no
// Python runs on the host (PREREG §7.3).
type BoxExtractor struct {
	Runner box.Runner
}

// Extract runs the helper over the project, mounted read-only.
func (e BoxExtractor) Extract(ctx context.Context, projectDir, equippedCommit string) (Snapshot, error) {
	result, err := e.Runner.Run(ctx, box.Command{
		Argv:   []string{pythonBinary, isolatedFlag, commandFlag, helperSource, projectDir, equippedCommit},
		Env:    containerEnv,
		Dir:    projectDir,
		Mounts: []box.Mount{{Path: projectDir, ReadOnly: true}},
		Limit:  extractLimit,
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrExtractFailed, err)
	}
	if result.ExitCode != 0 || result.TimedOut || result.OOMKilled {
		return Snapshot{}, fmt.Errorf("%w: exit %d: %s", ErrExtractFailed, result.ExitCode, excerpt(result.Stderr))
	}
	var snapshot Snapshot
	if err := json.Unmarshal(result.Stdout, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrExtractOutput, err)
	}
	return snapshot, nil
}

func excerpt(text []byte) string {
	if len(text) > stderrExcerpt {
		text = text[len(text)-stderrExcerpt:]
	}
	return string(text)
}
