package oracle

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The git invocation that reads a path of a commit or tree out of the run's
// repositories. It runs in the box, never on the host.
const (
	gitBinary         = "git"
	gitDirFlag        = "--git-dir"
	gitUnsafeDirFlag  = "-c"
	gitUnsafeDirValue = "safe.directory=*"
	gitArchive        = "archive"
	gitArchiveFormat  = "--format=tar"
	gitArchiveOutput  = "-o"
	envObjectDir      = "GIT_OBJECT_DIRECTORY"
	envAlternates     = "GIT_ALTERNATE_OBJECT_DIRECTORIES"
	pathspecMissing   = "did not match any files"
	tarSuffix         = ".tar"
)

// snapshotReader reads paths of the run's repositories through the box.
type snapshotReader struct {
	runner box.Runner
	runDir string
	// mountRoot is the directory the container may write; it holds the archives.
	mountRoot string
}

// source says where a tree-ish is stored: in the project's own repository, or in
// the separate object store of the stop snapshots, which borrows the project's
// objects.
type source int

const (
	fromProject source = iota
	fromStopStore
)

func (s snapshotReader) gitDir() string { return filepath.Join(projectPath(s.runDir), repositoryDir) }

func (s snapshotReader) environment(from source) []string {
	env := append([]string{}, boxEnv...)
	if from == fromStopStore {
		env = append(env,
			envObjectDir+"="+filepath.Join(s.runDir, study.StopObjectsDir),
			envAlternates+"="+filepath.Join(s.gitDir(), "objects"))
	}
	return env
}

// extract writes the pathspec of the tree-ish under dest, keeping its relative
// path. It reports false, and writes nothing, when the tree-ish holds no such
// path.
func (s snapshotReader) extract(ctx context.Context, from source, treeish, pathspec, dest string) (bool, error) {
	archive := dest + tarSuffix
	defer os.Remove(archive)
	result, err := s.runner.Run(ctx, box.Command{
		Argv: []string{
			gitBinary, gitUnsafeDirFlag, gitUnsafeDirValue, gitDirFlag, s.gitDir(),
			gitArchive, gitArchiveFormat, gitArchiveOutput, archive, treeish, pathspec,
		},
		Env:    s.environment(from),
		Dir:    s.runDir,
		Mounts: []box.Mount{{Path: s.runDir, ReadOnly: true}, {Path: s.mountRoot}},
	})
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrBox, err)
	}
	if result.ExitCode != 0 {
		if bytes.Contains(result.Stderr, []byte(pathspecMissing)) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %s %s: exit %d: %s", ErrArchive, treeish, pathspec, result.ExitCode, strings.TrimSpace(string(result.Stderr)))
	}
	if err := untar(archive, dest); err != nil {
		return false, err
	}
	return true, nil
}

// untar extracts the regular files of a tar archive under dest, refusing a name
// that leaves it.
func untar(archive, dest string) error {
	file, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrArchive, err)
	}
	defer file.Close()
	reader := tar.NewReader(file)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrArchive, err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := path.Clean(header.Name)
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("%w: entry %q leaves the destination", ErrArchive, header.Name)
		}
		if err := writeFrom(reader, filepath.Join(dest, filepath.FromSlash(name)), fs.FileMode(header.Mode).Perm()); err != nil {
			return err
		}
	}
}

func writeFrom(reader io.Reader, target string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|fileMode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, reader); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
