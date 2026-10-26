package prove

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/completeness/internal/app/ports"
	"github.com/RachidChabane/musthold/completeness/internal/domain/code"
	"github.com/RachidChabane/musthold/completeness/internal/domain/hunk"
	"github.com/RachidChabane/musthold/format"
)

// Findings the run reports about the change.
const (
	findingNoBase          = "no_base"
	findingUnreadableBase  = "unreadable_base"
	findingHunksUnreviewed = "hunks_unreviewed"
)

// changeSummary is what the change since the base did to the source of the
// project: the base commit, the changed units and the digest of each file that
// holds one.
type changeSummary struct {
	base  string
	hunks []hunk.Hunk
	// files maps a changed file to the digest of its content.
	files map[string]string
}

// loadChanges reads the change since the base and finds its hunks. A project that
// is not under version control has no change to compare, which is reported and
// leaves the diff signals out.
func (p *passState) loadChanges(ctx context.Context) error {
	p.changes = changeSummary{files: map[string]string{}}
	set, err := p.deps.Changes.Changes(ctx, p.options.Base)
	if errors.Is(err, ports.ErrNoRepository) {
		p.addFinding(fmt.Sprintf("%s: %v", findingNoBase, err))
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the change: %w", err)
	}
	p.changes.base = set.Base
	for _, change := range set.Changes {
		p.hunksOf(change)
	}
	slices.SortFunc(p.changes.hunks, func(a, b hunk.Hunk) int {
		return cmp.Or(strings.Compare(a.Unit.File, b.Unit.File), cmp.Compare(a.Unit.StartLine, b.Unit.StartLine), strings.Compare(a.Unit.Qualname, b.Unit.Qualname))
	})
	return nil
}

// reviewable reports whether a changed file is source the diff signals read: a
// file an indexer serves that holds no tests, lies outside the test roots and is
// not a spec file.
func (p *passState) reviewable(file string) (ports.Indexer, bool) {
	indexer, handled := p.indexerFor(file)
	if !handled || indexer.IsTest(file) {
		return nil, false
	}
	for _, root := range p.project.TestRoots {
		if file == root || strings.HasPrefix(file, root+"/") {
			return nil, false
		}
	}
	for _, spec := range p.project.Specs {
		if spec.Path == file {
			return nil, false
		}
	}
	return indexer, true
}

func (p *passState) hunksOf(change ports.Change) {
	file := path.Clean(change.Path)
	indexer, reviewable := p.reviewable(file)
	if !reviewable || change.After == nil {
		return
	}
	after, err := indexer.Index(file, change.After)
	if err != nil {
		p.addFinding(fmt.Sprintf("%s %s: %v", findingUnreadableSource, file, err))
		return
	}
	var before code.File
	if change.Before != nil {
		if before, err = indexer.Index(path.Clean(change.OldPath), change.Before); err != nil {
			p.addFinding(fmt.Sprintf("%s %s: %v", findingUnreadableBase, change.OldPath, err))
		}
	}
	found := hunk.Between(before.Units, after.Units)
	if len(found) == 0 {
		return
	}
	p.changes.hunks = append(p.changes.hunks, found...)
	p.changes.files[file] = format.SHA256Hex(change.After)
}

// hunksIn lists the hunks of a file.
func (c changeSummary) hunksIn(file string) []hunk.Hunk {
	var found []hunk.Hunk
	for _, candidate := range c.hunks {
		if candidate.Unit.File == file {
			found = append(found, candidate)
		}
	}
	return found
}
