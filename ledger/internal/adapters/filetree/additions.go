package filetree

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/app"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/ledger"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/refusal"
)

// AdditionDir lists the files of a directory as ledger additions at
// Prefix + relative path. The prefix is empty for a directory that already has the
// ledger layout (the output of `sign`), and a ledger directory name for the
// mutants.
type AdditionDir struct {
	Root   string
	Prefix string
	Policy ledger.ConflictPolicy
}

// Additions lists every file below Root.
func (d AdditionDir) Additions() ([]ledger.Addition, error) {
	var additions []ledger.Addition
	err := walkFiles(d.Root, func(relative string, content []byte) error {
		additions = append(additions, ledger.Addition{Path: joinSlash(d.Prefix, relative), Content: content, Policy: d.Policy})
		return nil
	})
	return additions, err
}

// TrustSnapshotDir lists a TUF repository snapshot as ledger additions below
// `trust/<snapshot id>/`, the id being the digest of the snapshot's timestamp file.
// Every file but the timestamp file keeps its content for good (versioned metadata
// and targets named by their digest), and the timestamp file is what a refresh of
// the repository replaces, so each refresh lands at paths of its own.
type TrustSnapshotDir struct {
	Root string
}

// Additions lists every file of the snapshot. A directory without a timestamp file
// is not a snapshot and is refused.
func (d TrustSnapshotDir) Additions() ([]ledger.Addition, error) {
	timestamp, err := os.ReadFile(filepath.Join(d.Root, format.TrustTimestampFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, refusal.Newf("%s holds no %s: it is not a TUF snapshot", d.Root, format.TrustTimestampFile)
	}
	if err != nil {
		return nil, err
	}
	prefix := joinSlash(format.TrustDirectory, ledger.SnapshotID(timestamp))
	return AdditionDir{Root: d.Root, Prefix: prefix}.Additions()
}

// AdditionSources concatenates several sources, so one append takes the signed
// proofs, the mutants, the trust snapshot and the seed together.
type AdditionSources []app.AdditionSource

// Additions returns the additions of every source, in order.
func (s AdditionSources) Additions() ([]ledger.Addition, error) {
	var additions []ledger.Addition
	for _, source := range s {
		listed, err := source.Additions()
		if err != nil {
			return nil, err
		}
		additions = append(additions, listed...)
	}
	return additions, nil
}

// FixedAdditions is a source of additions known in advance, such as the seed
// record of a run.
type FixedAdditions []ledger.Addition

// Additions returns the additions.
func (f FixedAdditions) Additions() ([]ledger.Addition, error) {
	return f, nil
}
