// Package verifiedout writes the verified directory: the bare statements and the
// manifest. The directory is built beside its destination and swapped in, so a
// failed run leaves the previous directory untouched and a successful one leaves
// no file of the previous run behind.
package verifiedout

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/verify/internal/domain/proofset"
)

const (
	directoryMode fs.FileMode = 0o755
	fileMode      fs.FileMode = 0o644

	stagingPattern = ".verified-staging-"
)

// Directory is the verified directory, usually `.musthold/verified`.
type Directory struct {
	Path string
}

// Write replaces the directory by the statement files and the manifest.
func (d Directory) Write(files []proofset.File, manifest []byte) error {
	parent := filepath.Dir(d.Path)
	if err := os.MkdirAll(parent, directoryMode); err != nil {
		return fmt.Errorf("create %s: %w", parent, err)
	}
	staging, err := os.MkdirTemp(parent, stagingPattern)
	if err != nil {
		return fmt.Errorf("create the staging directory: %w", err)
	}
	defer os.RemoveAll(staging)
	for _, file := range files {
		if err := writeFile(filepath.Join(staging, filepath.FromSlash(file.Path)), file.Content); err != nil {
			return err
		}
	}
	if err := writeFile(filepath.Join(staging, proofset.ManifestFileName), manifest); err != nil {
		return err
	}
	if err := os.RemoveAll(d.Path); err != nil {
		return fmt.Errorf("remove the previous verified directory: %w", err)
	}
	if err := os.Rename(staging, d.Path); err != nil {
		return fmt.Errorf("install the verified directory: %w", err)
	}
	return os.Chmod(d.Path, directoryMode)
}

func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, fileMode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
