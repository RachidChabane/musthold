// Package filetree is the file-system side of the ledger module: the cache
// directory statements are read from, the directory signed bundles are written
// to, the directories whose files are added to the ledger, and the directory the
// ledger is exported into.
package filetree

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/ledger/internal/app"
	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
)

const (
	directoryMode fs.FileMode = 0o755
	fileMode      fs.FileMode = 0o644
)

// StatementDir reads the statements a module cached: `<module>/<subject key>/<cacheKey>.json`
// below Root. A file that has another name is refused, so nothing but
// statements is ever signed by accident.
type StatementDir struct {
	Root string
}

// Statements lists the statements of the directory in path order.
func (d StatementDir) Statements() ([]app.Statement, error) {
	var statements []app.Statement
	err := walkFiles(d.Root, func(relative string, content []byte) error {
		location, ok := format.ParseStatementPath(relative)
		if !ok {
			return refusal.Newf("%s is not <module>/<subject key>/<cacheKey>%s", relative, format.StatementSuffix)
		}
		statements = append(statements, app.Statement{Location: location, Bytes: content})
		return nil
	})
	return statements, err
}

// walkFiles calls visit with the slash-separated path relative to root and the
// content of every file below root, in lexical order. A directory entry that is
// neither a directory nor a regular file is refused.
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

// BundleDir stores signed bundles as `proofs/<module>/<subject key>/<cacheKey>.dsse.json`
// below Root, the layout of the ledger.
type BundleDir struct {
	Root string
}

// WriteBundles writes every bundle. A bundle that exists with other bytes is
// refused: the output directory of a run is never overwritten.
func (d BundleDir) WriteBundles(bundles []app.Bundle) error {
	for _, bundle := range bundles {
		target := filepath.Join(d.Root, filepath.FromSlash(bundle.Location.BundlePath()))
		if existing, err := os.ReadFile(target); err == nil {
			if string(existing) != string(bundle.Bytes) {
				return refusal.Newf("%s exists with other bytes", bundle.Location.BundlePath())
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), directoryMode); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, bundle.Bytes, fileMode); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
	}
	return nil
}

// joinSlash joins slash-separated path segments, ignoring empty ones.
func joinSlash(segments ...string) string {
	return path.Join(segments...)
}
