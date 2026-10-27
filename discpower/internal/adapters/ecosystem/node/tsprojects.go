package node

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// A TypeScript project is a tsconfig file of the workspace, named by its path
// relative to the workspace root in POSIX form; tsc checks it with
// `tsc --noEmit -p <path>` run at the workspace root.

const (
	// hiddenPrefix starts the name of a directory no project lives in.
	hiddenPrefix = "."
	// jsonExtension ends a reference that names a tsconfig file rather than
	// the directory holding one.
	jsonExtension = ".json"
)

// tsconfig is the part of a tsconfig file that tells which projects check its
// files: its own file list and patterns, and the projects it references.
type tsconfig struct {
	Files      *[]string `json:"files"`
	Include    *[]string `json:"include"`
	Extends    any       `json:"extends"`
	References []struct {
		Path string `json:"path"`
	} `json:"references"`
}

// solutionStyle reports whether the tsconfig checks no file of its own: an
// empty file list, no patterns and nothing it extends, so that only the
// projects it references hold files.
func (c tsconfig) solutionStyle() bool {
	return c.Files != nil && len(*c.Files) == 0 && c.Include == nil && c.Extends == nil
}

// owningProjects are the projects that check a file of the workspace: the
// nearest tsconfig.json from the file's directory up to the workspace root,
// or, when that tsconfig is solution-style, the projects it references, each
// solution-style reference replaced by its own references; none when no
// tsconfig.json lies on the way up.
func owningProjects(dir, file string) []string {
	for current := path.Dir(file); ; current = path.Dir(current) {
		candidate := path.Join(current, tsconfigFile)
		if isFile(inDirectory(dir, candidate)) {
			return checkedProjects(dir, candidate, map[string]bool{})
		}
		if current == rootDirectory || current == "/" || current == "" {
			return nil
		}
	}
}

// workspaceProjects are the projects of every tsconfig.json of the workspace
// outside its installed packages and hidden directories, each resolved as
// owningProjects resolves it, in sorted order without repetition.
func workspaceProjects(dir string) []string {
	var projects []string
	_ = fs.WalkDir(os.DirFS(dir), rootDirectory, func(name string, entry fs.DirEntry, walkErr error) error {
		switch {
		case walkErr != nil:
			return fs.SkipDir
		case entry.IsDir():
			if name != rootDirectory && (strings.HasPrefix(entry.Name(), hiddenPrefix) || slices.Contains(dependencyDirectories, entry.Name()) || slices.Contains(outputDirectories, entry.Name())) {
				return fs.SkipDir
			}
		case entry.Name() == tsconfigFile && entry.Type().IsRegular():
			projects = append(projects, checkedProjects(dir, name, map[string]bool{})...)
		}
		return nil
	})
	slices.Sort(projects)
	return slices.Compact(projects)
}

// checkedProjects resolves one tsconfig into the projects tsc checks for it:
// itself, or the projects a solution-style tsconfig references, recursively,
// each project once.
func checkedProjects(dir, config string, seen map[string]bool) []string {
	if seen[config] {
		return nil
	}
	seen[config] = true
	parsed, readable := readTSConfig(inDirectory(dir, config))
	if !readable || !parsed.solutionStyle() {
		return []string{config}
	}
	var projects []string
	for _, reference := range parsed.References {
		if referenced, inside := referencedConfig(dir, config, reference.Path); inside {
			projects = append(projects, checkedProjects(dir, referenced, seen)...)
		}
	}
	return projects
}

// referencedConfig is the tsconfig a reference of a project names: the file
// itself when it names a JSON file, else the tsconfig.json of the directory;
// false when it lies outside the workspace or does not exist.
func referencedConfig(dir, config, reference string) (string, bool) {
	target := path.Clean(path.Join(path.Dir(config), filepath.ToSlash(reference)))
	if !strings.HasSuffix(target, jsonExtension) {
		target = path.Join(target, tsconfigFile)
	}
	if target == parentDirectory || strings.HasPrefix(target, parentDirectory+"/") || path.IsAbs(target) {
		return "", false
	}
	return target, isFile(inDirectory(dir, target))
}

// readTSConfig reads a tsconfig file, which may hold comments and trailing
// commas; false when it cannot be read as one.
func readTSConfig(file string) (tsconfig, bool) {
	content, err := os.ReadFile(file)
	if err != nil {
		return tsconfig{}, false
	}
	var parsed tsconfig
	if json.Unmarshal(standardJSON(content), &parsed) != nil {
		return tsconfig{}, false
	}
	return parsed, true
}

// standardJSON removes the comments and the trailing commas a tsconfig may
// hold, leaving strings as they are.
func standardJSON(content []byte) []byte {
	var out []byte
	inString, escaped := false, false
	for index := 0; index < len(content); index++ {
		char := content[index]
		switch {
		case inString:
			out = append(out, char)
			switch {
			case escaped:
				escaped = false
			case char == '\\':
				escaped = true
			case char == '"':
				inString = false
			}
		case char == '"':
			inString = true
			out = append(out, char)
		case char == '/' && index+1 < len(content) && content[index+1] == '/':
			for index < len(content) && content[index] != '\n' {
				index++
			}
			out = append(out, '\n')
		case char == '/' && index+1 < len(content) && content[index+1] == '*':
			end := strings.Index(string(content[index+2:]), "*/")
			if end < 0 {
				return out
			}
			index += end + 3
		case char == '}' || char == ']':
			out = append(trimTrailingComma(out), char)
		default:
			out = append(out, char)
		}
	}
	return out
}

// trimTrailingComma drops a comma that only white space separates from the
// end of the output.
func trimTrailingComma(out []byte) []byte {
	end := len(out)
	for end > 0 && strings.ContainsRune(" \t\r\n", rune(out[end-1])) {
		end--
	}
	if end > 0 && out[end-1] == ',' {
		return append(out[:end-1], out[end:]...)
	}
	return out
}

// inDirectory is the path on disk of a POSIX path relative to the directory.
func inDirectory(dir, name string) string {
	return filepath.Join(dir, filepath.FromSlash(name))
}
