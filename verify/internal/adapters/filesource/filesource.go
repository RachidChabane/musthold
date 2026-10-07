// Package filesource reads the inputs of the verifier from files: the exported
// ledger (`proofs/` and `trust/`), a module's local cache and the pending files.
// It never runs a process and never opens a connection.
package filesource

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/verify/internal/app"
	"github.com/RachidChabane/spec-proof/verify/internal/domain/refusal"
)

// walkFiles calls visit with the slash-separated path relative to root and the
// content of every file below root, in lexical order. Anything that is neither a
// directory nor a regular file, a symbolic link included, is refused.
func walkFiles(root string, visit func(relative string, content []byte) error) error {
	return filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		slashed := filepath.ToSlash(relative)
		if !entry.Type().IsRegular() {
			return refusal.Newf("%s is not a regular file", slashed)
		}
		content, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		return visit(slashed, content)
	})
}

// ExportedLedger is a directory written by `ledger export`: signed proofs below
// `proofs/` and the trust snapshot below `trust/`.
type ExportedLedger struct {
	Root string
}

// Bundles lists the signed proofs. A file outside `proofs/` and `trust/`, or a file
// of `proofs/` that is not at a proof path, is refused: nothing in the directory
// goes unexamined.
func (l ExportedLedger) Bundles() ([]app.BundleFile, error) {
	var bundles []app.BundleFile
	err := walkFiles(l.Root, func(relative string, content []byte) error {
		first, _, _ := strings.Cut(relative, "/")
		switch first {
		case format.TrustDirectory:
			return nil
		case format.ProofsDirectory:
			location, ok := format.ParseBundlePath(relative)
			if !ok {
				return refusal.Newf("%s is not %s/<module>/<subject key>/<cacheKey>%s", relative, format.ProofsDirectory, format.BundleSuffix)
			}
			bundles = append(bundles, app.BundleFile{Location: location, Bytes: content})
			return nil
		default:
			return refusal.Newf("%s is neither a proof nor a file of the trust snapshot", relative)
		}
	})
	return bundles, err
}

// LocalCache is a module's local cache: `<module>/<subject key>/<cacheKey>.json`.
type LocalCache struct {
	Root string
}

// LocalStatements lists the cached statements. A file that is not at a statement
// path is refused.
func (c LocalCache) LocalStatements() ([]app.LocalStatement, error) {
	var statements []app.LocalStatement
	err := walkFiles(c.Root, func(relative string, content []byte) error {
		location, ok := format.ParseStatementPath(relative)
		if !ok {
			return refusal.Newf("%s is not <module>/<subject key>/<cacheKey>%s", relative, format.StatementSuffix)
		}
		statements = append(statements, app.LocalStatement{Location: location, Bytes: content})
		return nil
	})
	return statements, err
}

// PendingDir is `.spec-proof/pending`: one `<module>.json` per module.
type PendingDir struct {
	Root string
}

// Pending reads the pending files. An absent directory means no module waits on
// anything. A file whose module is not its name is refused.
func (d PendingDir) Pending() ([]format.Pending, error) {
	entries, err := os.ReadDir(d.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pending []format.Pending
	for _, entry := range entries {
		name := entry.Name()
		module, isPending := strings.CutSuffix(name, format.PendingFileExtension)
		if entry.IsDir() || !isPending {
			return nil, refusal.Newf("%s is not <module>%s", name, format.PendingFileExtension)
		}
		content, err := os.ReadFile(filepath.Join(d.Root, name))
		if err != nil {
			return nil, err
		}
		file, err := format.DecodePending(content)
		if err != nil {
			return nil, refusal.Newf("pending file %s: %v", name, err)
		}
		if file.Module != module {
			return nil, refusal.Newf("pending file %s is about module %q", name, file.Module)
		}
		pending = append(pending, file)
	}
	return pending, nil
}
