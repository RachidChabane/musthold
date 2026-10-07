package prove

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/code"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/reach"
)

// Findings the run reports about the sources.
const (
	findingUnreadableSource = "unreadable_source"
	hiddenPrefix            = "."
)

// skippedDirectories are the directories no source of the project lives in.
var skippedDirectories = []string{"vendor", "node_modules", "__pycache__", "testdata", "venv", "site-packages"}

// indexerFor finds the indexer that serves a file.
func (p *passState) indexerFor(file string) (ports.Indexer, bool) {
	for _, indexer := range p.deps.Indexers {
		if indexer.Handles(file) {
			return indexer, true
		}
	}
	return nil, false
}

// loadSources indexes every source file of the project, tests included, so that
// reachability can pass through the helpers of a test, and builds the graph the
// justification reads.
func (p *passState) loadSources() error {
	var units []code.Unit
	err := fs.WalkDir(p.deps.Project, ".", func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if file != "." && (strings.HasPrefix(entry.Name(), hiddenPrefix) || slices.Contains(skippedDirectories, entry.Name())) {
				return fs.SkipDir
			}
			return nil
		}
		indexer, handled := p.indexerFor(file)
		if !handled {
			return nil
		}
		indexed, ok := p.indexFile(indexer, file)
		if ok {
			units = append(units, indexed.Units...)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk the project: %w", err)
	}
	p.units = units
	p.graph = reach.NewGraph(units)
	return nil
}

func (p *passState) indexFile(indexer ports.Indexer, file string) (code.File, bool) {
	content, err := fs.ReadFile(p.deps.Project, path.Clean(file))
	if err != nil {
		p.addFinding(fmt.Sprintf("%s %s: %v", findingUnreadableSource, file, err))
		return code.File{}, false
	}
	indexed, err := indexer.Index(file, content)
	if err != nil {
		p.addFinding(fmt.Sprintf("%s %s: %v", findingUnreadableSource, file, err))
		return code.File{}, false
	}
	return indexed, true
}
