package product

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
)

// ErrNotRegular refuses a tree entry that is neither a directory nor a regular file.
var ErrNotRegular = errors.New("tree entry is not a regular file")

// FileDigest is the SHA-256 of a file's content, in hexadecimal.
func FileDigest(fsys fs.FS, name string) (string, error) {
	file, err := fsys.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// TreeDigest is the digest of a directory tree: the SHA-256 of one line
// "<file digest>  <slash-separated path>\n" for each regular file, walked in
// lexical order. It depends on paths and content alone, never on modes, times or
// the order the files were written. A symbolic link or special file is refused.
func TreeDigest(fsys fs.FS) (string, error) {
	var listing strings.Builder
	err := fs.WalkDir(fsys, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%w: %s", ErrNotRegular, path)
		}
		digest, err := FileDigest(fsys, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(&listing, "%s  %s\n", digest, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(listing.String()))
	return hex.EncodeToString(sum[:]), nil
}
