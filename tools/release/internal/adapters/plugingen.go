package adapters

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/musthold/tools/release/internal/domain"
)

const snapshotSeparator = "\n--\n"

// PluginGenerator runs the plugin generators of the repository and reports
// whether they change any committed file.
type PluginGenerator struct {
	root string
	git  Git
}

// NewPluginGenerator returns a generator check for the repository rooted at root.
func NewPluginGenerator(root string) PluginGenerator {
	return PluginGenerator{root: root, git: NewGit(root)}
}

// PluginGenerated runs script/plugin-gen and fails when it changes the files of
// the module or the marketplace.
func (p PluginGenerator) PluginGenerated(module string) domain.Outcome {
	script := filepath.Join(p.root, domain.PluginGenScript)
	if _, err := os.Stat(script); errors.Is(err, os.ErrNotExist) {
		return domain.Outcome{Ran: true, Failure: domain.PluginGenScript + " does not exist"}
	}
	paths := []string{module, domain.MarketplaceFile}
	before, err := p.snapshot(paths)
	if err != nil {
		return domain.Outcome{Ran: true, Failure: err.Error()}
	}
	runner := commandRunner{dir: p.root}
	if output, err := runner.run(nil, script); err != nil {
		return domain.Outcome{Ran: true, Failure: failureText(output, err)}
	}
	after, err := p.snapshot(paths)
	if err != nil {
		return domain.Outcome{Ran: true, Failure: err.Error()}
	}
	if before != after {
		return domain.Outcome{Ran: true, Failure: "the working tree differs after the run:\n" + strings.TrimSpace(after)}
	}
	return domain.Outcome{Ran: true}
}

// snapshot records the state of paths: their status and their uncommitted diff.
func (p PluginGenerator) snapshot(paths []string) (string, error) {
	arguments := append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, paths...)
	status, err := p.git.output(arguments...)
	if err != nil {
		return "", err
	}
	diff, err := p.git.output(append([]string{"diff", "--"}, paths...)...)
	if err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}
	return status + snapshotSeparator + diff, nil
}
