package hostinstall

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrUnsupportedYAML is returned for a document whose shape the line-level edits
// cannot extend without rewriting it: the edit is left to the user.
var ErrUnsupportedYAML = errors.New("unsupported YAML layout")

const (
	indentUnit   = "  "
	listMarker   = "- "
	flowOpen     = "["
	flowClose    = "]"
	flowSplitter = ", "
	newline      = "\n"

	// yamlIndicators are the characters that give a plain scalar a meaning when it
	// starts with one.
	yamlIndicators = "*&!|>'\"%@`[]{},#?-: "
)

// yamlText is a YAML document edited line by line. yaml.v3 only locates nodes;
// the edit inserts, removes or replaces whole lines and never re-encodes the
// document, so every byte the edit does not concern, comments and blank lines
// included, stays as it was.
type yamlText struct {
	lines []string
	root  *yaml.Node
}

func parseYAML(content []byte) (*yamlText, error) {
	text := &yamlText{}
	if len(strings.TrimSpace(string(content))) > 0 {
		var document yaml.Node
		if err := yaml.Unmarshal(content, &document); err != nil {
			return nil, fmt.Errorf("parse YAML: %w", err)
		}
		if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%w: the document is not a mapping", ErrUnsupportedYAML)
		}
		text.root = document.Content[0]
		text.lines = strings.Split(strings.TrimSuffix(string(content), newline), newline)
	}
	return text, nil
}

func (t *yamlText) bytes() []byte {
	if len(t.lines) == 0 {
		return nil
	}
	return []byte(strings.Join(t.lines, newline) + newline)
}

// child returns the key and value nodes of a member of a mapping.
func child(mapping *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index], mapping.Content[index+1]
		}
	}
	return nil, nil
}

// lastLine is the last line a node occupies.
func lastLine(node *yaml.Node) int {
	last := node.Line
	for _, content := range node.Content {
		last = max(last, lastLine(content))
	}
	return last
}

// indentOf is the indentation of a line, in spaces.
func indentOf(line string) int { return len(line) - len(strings.TrimLeft(line, " ")) }

func (t *yamlText) insertAfter(line int, inserted ...string) {
	t.lines = slices.Insert(t.lines, line, inserted...)
}

func (t *yamlText) remove(line int) { t.lines = slices.Delete(t.lines, line-1, line) }

// blockMapping returns the mapping a key holds, requiring block style.
func blockMapping(parent *yaml.Node, key string) (*yaml.Node, *yaml.Node, error) {
	keyNode, value := child(parent, key)
	if value == nil {
		return nil, nil, nil
	}
	if value.Kind != yaml.MappingNode || value.Style&yaml.FlowStyle != 0 {
		return nil, nil, fmt.Errorf("%w: %q is not a block mapping", ErrUnsupportedYAML, key)
	}
	return keyNode, value, nil
}

// addToList makes sure `path` is a list of strings holding item, creating what is
// missing at the end of its parent. It reports whether the document changed.
func (t *yamlText) addToList(parent, key, item string) (bool, error) {
	if t.root == nil {
		t.lines = []string{parent + ":", indentUnit + key + ":", indentUnit + indentUnit + listMarker + item}
		return true, nil
	}
	_, mapping, err := blockMapping(t.root, parent)
	if err != nil {
		return false, err
	}
	if mapping == nil {
		t.lines = append(t.lines, parent+":", indentUnit+key+":", indentUnit+indentUnit+listMarker+item)
		return true, nil
	}
	keyNode, list := child(mapping, key)
	if list == nil {
		indent := strings.Repeat(" ", indentOf(t.lines[mapping.Content[0].Line-1]))
		t.insertAfter(lastLine(mapping), indent+key+":", indent+indentUnit+listMarker+item)
		return true, nil
	}
	return t.extendList(keyNode, list, item)
}

func (t *yamlText) extendList(keyNode, list *yaml.Node, item string) (bool, error) {
	switch {
	case list.Kind == yaml.ScalarNode && list.Tag == "!!null":
		indent := strings.Repeat(" ", indentOf(t.lines[keyNode.Line-1]))
		t.insertAfter(keyNode.Line, indent+indentUnit+listMarker+item)
		return true, nil
	case list.Kind != yaml.SequenceNode:
		return false, fmt.Errorf("%w: %q is not a list", ErrUnsupportedYAML, keyNode.Value)
	case containsItem(list, item):
		return false, nil
	case list.Style&yaml.FlowStyle != 0:
		return true, t.rewriteFlow(list, append(items(list), item))
	}
	indent := strings.Repeat(" ", indentOf(t.lines[list.Content[0].Line-1]))
	t.insertAfter(lastLine(list), indent+listMarker+item)
	return true, nil
}

func items(list *yaml.Node) []string {
	values := make([]string, len(list.Content))
	for index, content := range list.Content {
		values[index] = content.Value
	}
	return values
}

func containsItem(list *yaml.Node, item string) bool { return slices.Contains(items(list), item) }

// rewriteFlow replaces a flow list that sits on one line.
func (t *yamlText) rewriteFlow(list *yaml.Node, values []string) error {
	if lastLine(list) != list.Line {
		return fmt.Errorf("%w: a list spread over several lines in flow style", ErrUnsupportedYAML)
	}
	line := t.lines[list.Line-1]
	start := list.Column - 1
	end := strings.Index(line[start:], flowClose)
	if end < 0 {
		return fmt.Errorf("%w: a flow list that does not close on its line", ErrUnsupportedYAML)
	}
	t.lines[list.Line-1] = line[:start] + flowOpen + strings.Join(values, flowSplitter) + flowClose + line[start+end+1:]
	return nil
}

// removeFromList removes item from the list, and the key and the parent mapping
// when they end up empty. It reports whether the document changed.
func (t *yamlText) removeFromList(parent, key, item string) (bool, error) {
	if t.root == nil {
		return false, nil
	}
	parentKey, mapping, err := blockMapping(t.root, parent)
	if err != nil || mapping == nil {
		return false, err
	}
	keyNode, list := child(mapping, key)
	if list == nil || list.Kind != yaml.SequenceNode || !containsItem(list, item) {
		return false, nil
	}
	remaining := slices.DeleteFunc(items(list), func(value string) bool { return value == item })
	flow := list.Style&yaml.FlowStyle != 0
	switch {
	case len(remaining) > 0 && flow:
		return true, t.rewriteFlow(list, remaining)
	case len(remaining) > 0:
		for _, content := range list.Content {
			if content.Value == item {
				t.remove(content.Line)
				break
			}
		}
		return true, nil
	case len(mapping.Content) == 2:
		t.removeLines(parentKey.Line, lastLine(mapping))
	default:
		t.removeLines(keyNode.Line, lastLine(list))
	}
	return true, nil
}

// removeLines removes the lines from first to last, both 1-based and inclusive.
func (t *yamlText) removeLines(first, last int) {
	t.lines = slices.Delete(t.lines, first-1, last)
}

// setBool sets `parent.key` to the value, creating what is missing at the end of
// its parent. It reports whether the document changed.
func (t *yamlText) setBool(parent, key string, value bool) (bool, error) {
	text := fmt.Sprint(value)
	if t.root == nil {
		t.lines = []string{parent + ":", indentUnit + key + ": " + text}
		return true, nil
	}
	_, mapping, err := blockMapping(t.root, parent)
	if err != nil {
		return false, err
	}
	if mapping == nil {
		t.lines = append(t.lines, parent+":", indentUnit+key+": "+text)
		return true, nil
	}
	_, scalar := child(mapping, key)
	if scalar == nil {
		indent := strings.Repeat(" ", indentOf(t.lines[mapping.Content[0].Line-1]))
		t.insertAfter(lastLine(mapping), indent+key+": "+text)
		return true, nil
	}
	if scalar.Kind != yaml.ScalarNode || scalar.Tag != "!!bool" {
		return false, fmt.Errorf("%w: %s.%s is not a boolean", ErrUnsupportedYAML, parent, key)
	}
	if scalar.Value == text {
		return false, nil
	}
	line := t.lines[scalar.Line-1]
	start := scalar.Column - 1
	t.lines[scalar.Line-1] = line[:start] + text + line[start+len(scalar.Value):]
	return true, nil
}

// setStringIfAbsent sets `parent.key` to the text when the document has no such
// member, creating what is missing at the end of its parent, and leaves a member
// that is there as it is, whatever its value. It reports whether the document
// changed.
func (t *yamlText) setStringIfAbsent(parent, key, value string) (bool, error) {
	if t.root == nil {
		t.lines = []string{parent + ":", indentUnit + key + ": " + value}
		return true, nil
	}
	_, mapping, err := blockMapping(t.root, parent)
	if err != nil {
		return false, err
	}
	if mapping == nil {
		t.lines = append(t.lines, parent+":", indentUnit+key+": "+value)
		return true, nil
	}
	if _, present := child(mapping, key); present != nil {
		return false, nil
	}
	indent := strings.Repeat(" ", indentOf(t.lines[mapping.Content[0].Line-1]))
	t.insertAfter(lastLine(mapping), indent+key+": "+value)
	return true, nil
}

// fillList makes `parent.key` the list of the values when the document has no such
// list or the list is empty, and leaves a list that has items as it is. It
// reports whether the document changed.
func (t *yamlText) fillList(parent, key string, values []string) (bool, error) {
	if len(values) == 0 || t.listLength(parent, key) > 0 {
		return false, nil
	}
	changed := false
	for _, value := range values {
		added, err := t.addToList(parent, key, yamlScalar(value))
		if err != nil {
			return false, err
		}
		changed = changed || added
		if err := t.reparse(); err != nil {
			return false, err
		}
	}
	return changed, nil
}

// yamlScalar spells a string as a YAML scalar: as it is when it reads back as the
// same string, in single quotes when it starts with a character YAML gives a
// meaning, as the `*` of a glob does.
func yamlScalar(value string) string {
	if value == "" || strings.ContainsAny(value[:1], yamlIndicators) || strings.Contains(value, ": ") || strings.Contains(value, " #") {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	return value
}

// listLength is the number of items of `parent.key`, zero when it is not a list.
func (t *yamlText) listLength(parent, key string) int {
	if t.root == nil {
		return 0
	}
	_, mapping := child(t.root, parent)
	_, list := child(mapping, key)
	if list == nil || list.Kind != yaml.SequenceNode {
		return 0
	}
	return len(list.Content)
}

// reparse reads the document again after an edit, so that the positions of its
// nodes match its lines.
func (t *yamlText) reparse() error {
	parsed, err := parseYAML(t.bytes())
	if err != nil {
		return err
	}
	*t = *parsed
	return nil
}

// hasKey reports whether the document has a member at its top level.
func (t *yamlText) hasKey(key string) bool {
	if t.root == nil {
		return false
	}
	_, value := child(t.root, key)
	return value != nil
}

// boolValue reads `parent.key`.
func (t *yamlText) boolValue(parent, key string) (value, found bool) {
	if t.root == nil {
		return false, false
	}
	_, mapping := child(t.root, parent)
	_, scalar := child(mapping, key)
	if scalar == nil || scalar.Kind != yaml.ScalarNode {
		return false, false
	}
	return scalar.Value == "true", true
}
