package jvm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/suiterun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

// ErrProbeFailed is returned when the probe program cannot be run or cannot
// answer a request.
var ErrProbeFailed = errors.New("probe failed")

const (
	classpathExtension = ".classpath"
	scriptExtension    = ".gradle"
	digestSeparator    = "\x00"
	privateFileMode    = 0o600
)

// classpathCache keeps the dependencies of a project by the digest of its build
// files, so that the mutants of one project, which differ in sources only, share
// one resolution.
type classpathCache struct {
	mu      sync.Mutex
	entries map[string][]classpathEntry
}

func newClasspathCache() *classpathCache {
	return &classpathCache{entries: map[string][]classpathEntry{}}
}

func (c *classpathCache) get(key string) ([]classpathEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, found := c.entries[key]
	return entries, found
}

func (c *classpathCache) put(key string, entries []classpathEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = entries
}

// classpath returns the test runtime classpath of the workspace, in the form the
// java launcher takes. The build must already have produced its outputs.
func (s *Strategy) classpath(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain) (string, error) {
	directory, err := filepath.EvalSymlinks(workspace.Dir)
	if err != nil {
		return "", fmt.Errorf("resolve the workspace: %w", err)
	}
	key := s.buildDigest(directory)
	entries, found := s.classpaths.get(key)
	if !found {
		if entries, err = s.resolveClasspath(ctx, workspace, directory, toolchain); err != nil {
			return "", err
		}
		s.classpaths.put(key, entries)
	}
	return renderClasspath(directory, entries), nil
}

// buildDigest identifies the dependency declarations of the project in a
// directory.
func (s *Strategy) buildDigest(directory string) string {
	hash := sha256.New()
	for _, file := range s.tool.buildFiles() {
		content, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(file)))
		if err != nil {
			continue
		}
		hash.Write([]byte(file + digestSeparator))
		hash.Write(content)
		hash.Write([]byte(digestSeparator))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *Strategy) resolveClasspath(ctx context.Context, workspace ecosystem.Workspace, directory string, toolchain ecosystem.Toolchain) ([]classpathEntry, error) {
	plan := s.tool.classpath()
	output := suiterun.ResultsFile(workspace, classpathExtension)
	script := ""
	if plan.script != nil {
		script = suiterun.ResultsFile(workspace, scriptExtension)
		if err := os.WriteFile(script, plan.script, privateFileMode); err != nil {
			return nil, fmt.Errorf("write the classpath script: %w", err)
		}
	}
	argv := append(append([]string{}, toolchain.Command...), plan.args(script, output)...)
	result, err := s.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Timeout: ProbeTimeout})
	if err != nil {
		return nil, fmt.Errorf("resolve the classpath: %w", err)
	}
	content, readErr := os.ReadFile(output)
	if readErr != nil || result.ExitCode != 0 || result.TimedOut {
		return nil, fmt.Errorf("%w: the classpath of the project: exit %d, timed out %t: %s",
			ErrProbeFailed, result.ExitCode, result.TimedOut, strings.TrimSpace(string(result.Stdout)+string(result.Stderr)))
	}
	return plan.entries(directory, string(content)), nil
}

func renderClasspath(directory string, entries []classpathEntry) string {
	elements := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.inWorkspace {
			elements = append(elements, filepath.Join(directory, filepath.FromSlash(entry.path)))
		} else {
			elements = append(elements, entry.path)
		}
	}
	return strings.Join(elements, string(filepath.ListSeparator))
}
