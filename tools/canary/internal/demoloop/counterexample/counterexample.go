// Package counterexample turns a mutant the discriminating-power module kept
// into the file it stands for: the project's source with each edited unit
// replaced by the mutant's source, so that a real diff tool can show the change.
// It is pure and places the top-level unit forms of Python, the language of the
// demonstration's fixture.
package counterexample

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Edit is one edit of a mutant as the module's vault keeps it.
type Edit struct {
	File      string `json:"file"`
	Qualname  string `json:"qualname"`
	Target    string `json:"target"`
	NewSource string `json:"new_source"`
}

// Mutant is the edit set of one mutant.
type Mutant struct {
	Edits []Edit `json:"edits"`
}

// Target kinds of an edit, as the vault spells them; empty is a definition.
const (
	targetDefinition       = ""
	targetDefinitionNamed  = "definition"
	targetModuleAssignment = "module_assignment"
)

// Errors of an edit the splicer cannot place.
var (
	ErrUnitNotFound = errors.New("the unit is not in the file")
	ErrUnsupported  = errors.New("the unit form is not one the splicer places")
)

// Decode reads the vault file of a mutant.
func Decode(data []byte) (Mutant, error) {
	var mutant Mutant
	err := json.Unmarshal(data, &mutant)
	return mutant, err
}

// Files lists the files the mutant edits, in first-seen order.
func (m Mutant) Files() []string {
	var files []string
	seen := map[string]bool{}
	for _, edit := range m.Edits {
		if !seen[edit.File] {
			seen[edit.File] = true
			files = append(files, edit.File)
		}
	}
	return files
}

// Apply returns the content of a file with every edit of the mutant on that
// file put in place of the unit it names.
func (m Mutant) Apply(file, content string) (string, error) {
	for _, edit := range m.Edits {
		if edit.File != file {
			continue
		}
		start, end, err := locate(content, edit)
		if err != nil {
			return "", fmt.Errorf("%s :: %s: %w", edit.File, edit.Qualname, err)
		}
		content = content[:start] + strings.TrimRight(edit.NewSource, "\n") + "\n" + content[end:]
	}
	return content, nil
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// locate finds the byte range of a top-level Python unit: a function or class
// definition with its decorators and body, or a module-level assignment with
// its continuation lines. The blank lines after the unit are not part of it.
func locate(content string, edit Edit) (int, int, error) {
	if !identifier.MatchString(edit.Qualname) {
		return 0, 0, ErrUnsupported
	}
	header, err := headerOf(edit)
	if err != nil {
		return 0, 0, err
	}
	lines := strings.SplitAfter(content, "\n")
	offset := 0
	for index, line := range lines {
		if !header(line) {
			offset += len(line)
			continue
		}
		start := offset
		for first := index; first > 0 && strings.HasPrefix(lines[first-1], "@"); first-- {
			start -= len(lines[first-1])
		}
		end, last := offset+len(line), offset+len(line)
		for _, next := range lines[index+1:] {
			if !continues(next) {
				break
			}
			end += len(next)
			if strings.TrimSpace(next) != "" {
				last = end
			}
		}
		return start, last, nil
	}
	return 0, 0, ErrUnitNotFound
}

func headerOf(edit Edit) (func(string) bool, error) {
	switch edit.Target {
	case targetDefinition, targetDefinitionNamed:
		return func(line string) bool {
			for _, keyword := range []string{"def ", "async def ", "class "} {
				if strings.HasPrefix(line, keyword+edit.Qualname+"(") || strings.HasPrefix(line, keyword+edit.Qualname+":") {
					return true
				}
			}
			return false
		}, nil
	case targetModuleAssignment:
		assignment := regexp.MustCompile(`^` + edit.Qualname + `\s*(:[^=]*)?=`)
		return assignment.MatchString, nil
	default:
		return nil, ErrUnsupported
	}
}

// continues reports whether a line belongs to the unit above it: a blank or
// indented line, or one that closes a bracket of a multi-line expression.
func continues(line string) bool {
	trimmed := strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(trimmed) == "" {
		return true
	}
	return strings.HasPrefix(trimmed, " ") || strings.HasPrefix(trimmed, "\t") || strings.ContainsAny(trimmed[:1], ")]}")
}
