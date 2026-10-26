// Package witnesskit holds what the witness hosts of every ecosystem share:
// listing the test files of a project, choosing the public signatures a spec
// mentions, cutting a definition down to its header, and renaming a witness so
// that no two sets collide.
package witnesskit

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// MaxInterfaceEntries bounds the signatures a writer is shown when the spec
// mentions none of them and every public one stands in.
const MaxInterfaceEntries = 200

// RenamedPrefix starts the name every witness test is renamed to; the short id
// of the witness follows.
const RenamedPrefix = "musthold_witness_"

const (
	rootDirectory     = "."
	stateDirectory    = ".musthold"
	versionControlDir = ".git"
)

var identifierRegexp = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// TestFiles lists, sorted, the files of a project tree the layout names test
// files or that extra accepts, outside the excluded, dependency and state
// directories.
func TestFiles(project fs.FS, layout ecosystem.Layout, extra func(string) bool) ([]string, error) {
	skipped := slices.Concat(layout.ExcludedDirs, layout.DependencyDirs, []string{stateDirectory, versionControlDir})
	var files []string
	err := fs.WalkDir(project, rootDirectory, func(file string, entry fs.DirEntry, walkErr error) error {
		switch {
		case walkErr != nil:
			return walkErr
		case entry.IsDir() && file != rootDirectory && slices.Contains(skipped, path.Base(file)):
			return fs.SkipDir
		case entry.IsDir():
			return nil
		case layout.IsTestFile(file) || (extra != nil && extra(file)):
			files = append(files, file)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list the test files: %w", err)
	}
	slices.Sort(files)
	return files, nil
}

// Mentions returns whether a name is one of the identifiers of the text.
func Mentions(text string) func(string) bool {
	tokens := map[string]bool{}
	for _, token := range identifierRegexp.FindAllString(text, -1) {
		tokens[token] = true
	}
	return func(name string) bool { return tokens[name] }
}

// Entry is one public signature of the code under test.
type Entry struct {
	// Names are the identifiers a spec may mention it by: its own name and the
	// name of its owner.
	Names     []string
	Signature string
}

// Choose keeps the entries the spec mentions by one of their names, in order;
// when it mentions none, every entry stands in, at most MaxInterfaceEntries.
func Choose(entries []Entry, mentioned func(string) bool) []string {
	var chosen []string
	for _, entry := range entries {
		if slices.ContainsFunc(entry.Names, mentioned) {
			chosen = append(chosen, entry.Signature)
		}
	}
	if len(chosen) > 0 {
		return chosen
	}
	for _, entry := range entries {
		if len(chosen) == MaxInterfaceEntries {
			break
		}
		chosen = append(chosen, entry.Signature)
	}
	return chosen
}

// Header cuts a definition down to its header: the text up to the first
// terminator met outside brackets and quoted text, without it. A definition with
// no such terminator is returned whole, trimmed.
func Header(text string, terminator byte) string {
	depth := 0
	var quote byte
	for index := 0; index < len(text); index++ {
		char := text[index]
		switch {
		case quote != 0:
			if char == '\\' {
				index++
			} else if char == quote {
				quote = 0
			}
		case char == '"' || char == '\'' || char == '`':
			quote = char
		case char == '(' || char == '[' || (char == '{' && terminator != '{'):
			depth++
		case char == ')' || char == ']' || (char == '}' && terminator != '{'):
			depth--
		case char == '{' && depth > 0:
			depth++
		case char == '}' && depth > 0:
			depth--
		case char == terminator && depth == 0:
			return strings.TrimSpace(text[:index])
		}
	}
	return strings.TrimSpace(text)
}

// Rename renames the first declaration of a function in a witness's code: the
// declaration pattern's first submatch is the name. It reports whether the code
// declares it.
func Rename(code string, declaration *regexp.Regexp, renamed string) (string, bool) {
	location := declaration.FindStringSubmatchIndex(code)
	if location == nil || len(location) < 4 {
		return code, false
	}
	return code[:location[2]] + renamed + code[location[3]:], true
}

// Declaration builds the pattern of a declaration of the named function: the
// keyword at the start of a line, then the name, then its parameter list.
func Declaration(keyword, name string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(keyword) + `[ \t]+(` + regexp.QuoteMeta(name) + `)[ \t]*\(`)
}

// RenamedName is the name a witness test is renamed to.
func RenamedName(prefix, shortID string) string { return prefix + RenamedPrefix + shortID }

// LineOf returns the 1-based line of a byte offset of a text.
func LineOf(text string, offset int) int { return 1 + strings.Count(text[:offset], "\n") }
