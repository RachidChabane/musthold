package golang

import (
	"io/fs"
	"path"
	"strconv"
	"strings"
)

const (
	goModFile        = "go.mod"
	moduleDirective  = "module"
	lineComment      = "//"
	projectRootDir   = "."
	filesystemRoot   = "/"
	minimumModuleLen = 2
)

// moduleInfo locates the Go module a test file belongs to.
type moduleInfo struct {
	path string
	dir  string
}

// findModule returns the module of the nearest go.mod at or above the directory,
// or the zero value when the tree holds none.
func findModule(project fs.FS, startDir string) moduleInfo {
	for dir := startDir; ; dir = path.Dir(dir) {
		if data, err := fs.ReadFile(project, path.Join(dir, goModFile)); err == nil {
			if modulePath := parseModulePath(string(data)); modulePath != "" {
				return moduleInfo{path: modulePath, dir: dir}
			}
		}
		if dir == projectRootDir || dir == filesystemRoot {
			return moduleInfo{}
		}
	}
}

func parseModulePath(goMod string) string {
	for _, line := range strings.Split(goMod, "\n") {
		line, _, _ = strings.Cut(line, lineComment)
		fields := strings.Fields(line)
		if len(fields) >= minimumModuleLen && fields[0] == moduleDirective {
			return unquoteModule(fields[1])
		}
	}
	return ""
}

func unquoteModule(raw string) string {
	if unquoted, err := strconv.Unquote(raw); err == nil {
		return unquoted
	}
	return raw
}

// directoryOfImport maps an import path of the module to its project directory.
func (m moduleInfo) directoryOfImport(importPath string) (string, bool) {
	if m.path == "" {
		return "", false
	}
	if importPath == m.path {
		return m.dir, true
	}
	relative, inside := strings.CutPrefix(importPath, m.path+"/")
	if !inside {
		return "", false
	}
	return path.Join(m.dir, relative), true
}
