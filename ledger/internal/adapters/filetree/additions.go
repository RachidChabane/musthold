package filetree

import (
	"github.com/RachidChabane/spec-proof/ledger/internal/app"
	"github.com/RachidChabane/spec-proof/ledger/internal/domain/ledger"
)

// AdditionDir lists the files of a directory as ledger additions at
// Prefix + relative path. The prefix is empty for a directory that already has the
// ledger layout (the output of `sign`), and a ledger directory name for the
// mutants and the trust snapshot.
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
