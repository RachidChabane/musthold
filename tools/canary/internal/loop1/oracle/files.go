package oracle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// File modes of the files the verb creates.
const (
	dirMode  fs.FileMode = 0o755
	fileMode fs.FileMode = 0o644
)

// skipFunc tells, by slash-separated path relative to the copied root, which
// entries a copy or a digest leaves out.
type skipFunc func(rel string, entry fs.DirEntry) bool

func skipNothing(string, fs.DirEntry) bool { return false }

// skipTopLevel leaves out the named entries at the root of the tree. Entries
// named __pycache__ are left out at every depth.
func skipTopLevel(names ...string) skipFunc {
	return func(rel string, entry fs.DirEntry) bool {
		if entry.IsDir() && entry.Name() == pycacheDir {
			return true
		}
		return !strings.Contains(rel, "/") && slices.Contains(names, rel)
	}
}

// walkFiles calls visit for every regular file under root that skip keeps, in
// lexical order. Symbolic links and other special files are never followed or
// copied.
func walkFiles(root string, skip skipFunc, visit func(rel, abs string, mode fs.FileMode) error) error {
	return filepath.WalkDir(root, func(abs string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, abs)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if skip(rel, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return visit(rel, abs, info.Mode().Perm())
	})
}

// copyTree copies the regular files of src under dst, creating directories as
// needed and overwriting files that exist.
func copyTree(src, dst string, skip skipFunc) error {
	if err := os.MkdirAll(dst, dirMode); err != nil {
		return err
	}
	return walkFiles(src, skip, func(rel, abs string, mode fs.FileMode) error {
		return copyFile(abs, filepath.Join(dst, filepath.FromSlash(rel)), mode)
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), dirMode); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// digestTree is the SHA-256 of the kept files' paths, modes and contents.
func digestTree(root string, skip skipFunc) (string, error) {
	hash := sha256.New()
	err := walkFiles(root, skip, func(rel, abs string, mode fs.FileMode) error {
		fmt.Fprintf(hash, "%s\x00%o\x00", rel, mode)
		in, err := os.Open(abs)
		if err != nil {
			return err
		}
		defer in.Close()
		if _, err := io.Copy(hash, in); err != nil {
			return err
		}
		_, err = hash.Write([]byte{0})
		return err
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// digestParts combines named digests into one.
func digestParts(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		fmt.Fprintf(hash, "%d\x00%s\x00", len(part), part)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// listDirs lists the names of the directories directly under dir, sorted. A
// missing directory has none.
func listDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// testFiles lists the test files directly or deeply under dir as paths relative
// to prefix, sorted.
func testFiles(dir, prefix string) ([]string, error) {
	var files []string
	err := walkFiles(dir, skipTopLevel(), func(rel, _ string, _ fs.FileMode) error {
		if strings.HasPrefix(filepath.Base(rel), testFilePrefix) && strings.HasSuffix(rel, testFileSuffix) {
			files = append(files, prefix+"/"+rel)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return files, err
}

// writeFileAtomic writes data to path through a sibling temporary file.
func writeFileAtomic(path string, data []byte) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, fileMode); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
