package golang

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"golang.org/x/mod/modfile"
)

// workspaceFile is the file that makes a directory a Go workspace.
const workspaceFile = "go.work"

// WorkspaceModules lists the module directories the Go workspace at the root of
// the tree uses, in the order its use directives give them, and reports whether
// the root holds a workspace file.
func WorkspaceModules(root fs.FS) ([]string, bool, error) {
	content, err := fs.ReadFile(root, workspaceFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	workspace, err := modfile.ParseWork(workspaceFile, content, nil)
	if err != nil {
		return nil, true, fmt.Errorf("read %s: %w", workspaceFile, err)
	}
	modules := make([]string, 0, len(workspace.Use))
	for _, use := range workspace.Use {
		modules = append(modules, path.Clean(use.Path))
	}
	return modules, true, nil
}
