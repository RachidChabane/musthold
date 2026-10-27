package plan

import (
	"errors"
	"fmt"
	"strings"
)

// Keys of Shallnot's configuration that SetRequire edits.
const (
	proofsKey      = "proofs:"
	requireKey     = "require:"
	listItemPrefix = "- "
	commentPrefix  = "#"
	defaultIndent  = "  "
)

// ErrRequireShape refuses a Shallnot configuration whose `proofs` block SetRequire
// cannot locate: none, two, or one written on its own line as a flow mapping.
var ErrRequireShape = errors.New("shallnot.yaml has no `proofs` block of the expected shape")

// SetRequire writes `proofs.require` of Shallnot's configuration as the flow list
// of the kinds given, `[]` when there are none, in place of whatever list the
// block held, block or flow, and leaves every other line as it was.
func SetRequire(current []byte, kinds []string) ([]byte, error) {
	lines := strings.Split(string(current), "\n")
	start, err := proofsBlock(lines)
	if err != nil {
		return nil, err
	}
	end := blockEnd(lines, start)
	indent := childIndent(lines[start+1 : end])
	entry := indent + requireKey + " [" + strings.Join(kinds, ", ") + "]"
	at, found := requireLine(lines[start+1:end], indent)
	if !found {
		return []byte(strings.Join(insert(lines, start+1, entry), "\n")), nil
	}
	at += start + 1
	last := listEnd(lines, at, end, indent)
	edited := append(append(append([]string{}, lines[:at]...), entry), lines[last:]...)
	return []byte(strings.Join(edited, "\n")), nil
}

// proofsBlock finds the one top-level `proofs:` line that opens a block.
func proofsBlock(lines []string) (int, error) {
	found := -1
	for index, line := range lines {
		key, rest, ok := strings.Cut(line, ":")
		if !ok || key != strings.TrimSuffix(proofsKey, ":") {
			continue
		}
		if value := strings.TrimSpace(rest); value != "" && !strings.HasPrefix(value, commentPrefix) {
			return 0, fmt.Errorf("%w: `proofs` holds %q on its own line", ErrRequireShape, value)
		}
		if found >= 0 {
			return 0, fmt.Errorf("%w: two `proofs` blocks", ErrRequireShape)
		}
		found = index
	}
	if found < 0 {
		return 0, fmt.Errorf("%w: no `proofs` block", ErrRequireShape)
	}
	return found, nil
}

// blockEnd is the first line after the block that opens at start: the next line
// that starts at the first column and is not blank or a comment.
func blockEnd(lines []string, start int) int {
	for index := start + 1; index < len(lines); index++ {
		line := lines[index]
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, commentPrefix) {
			return index
		}
	}
	return len(lines)
}

// childIndent is the indentation of the block's first key.
func childIndent(block []string) string {
	for _, line := range block {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed != "" && !strings.HasPrefix(trimmed, commentPrefix) {
			return line[:len(line)-len(trimmed)]
		}
	}
	return defaultIndent
}

// requireLine finds the `require:` key at the block's indentation.
func requireLine(block []string, indent string) (int, bool) {
	for index, line := range block {
		if strings.HasPrefix(line, indent+requireKey) {
			return index, true
		}
	}
	return 0, false
}

// listEnd is the first line after the value of the key at line at: a block
// list's items sit at the key's indentation or deeper. Blank lines after the
// last item are kept.
func listEnd(lines []string, at, end int, indent string) int {
	last := at + 1
	for index := at + 1; index < end; index++ {
		line := lines[index]
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" {
			continue
		}
		depth := len(line) - len(trimmed)
		item := depth == len(indent) && strings.HasPrefix(trimmed, listItemPrefix)
		if depth <= len(indent) && !item {
			break
		}
		last = index + 1
	}
	return last
}

func insert(lines []string, at int, line string) []string {
	return append(append(append([]string{}, lines[:at]...), line), lines[at:]...)
}
