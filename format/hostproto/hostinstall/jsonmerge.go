package hostinstall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// Keys of the JSON files the installer merges into.
const (
	keyHooks          = "hooks"
	keyEnabledPlugins = "enabledPlugins"
)

func decodeObject(content []byte) (map[string]any, error) {
	object := map[string]any{}
	if len(bytes.TrimSpace(content)) == 0 {
		return object, nil
	}
	if err := json.Unmarshal(content, &object); err != nil {
		return nil, fmt.Errorf("not a JSON object: %w", err)
	}
	return object, nil
}

func encodeObject(object map[string]any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", indentUnit)
	if err := encoder.Encode(object); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func objectMember(parent map[string]any, key string) map[string]any {
	member, isObject := parent[key].(map[string]any)
	if !isObject {
		member = map[string]any{}
		parent[key] = member
	}
	return member
}

// mentions reports whether a JSON value holds the string anywhere.
func mentions(value any, text string) bool {
	switch typed := value.(type) {
	case string:
		return typed == text
	case []any:
		for _, item := range typed {
			if mentions(item, text) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if mentions(item, text) {
				return true
			}
		}
	}
	return false
}

// firstCommand finds the command of the first hook of a matcher group.
func firstCommand(group any) string {
	groupObject, _ := group.(map[string]any)
	hooks, _ := groupObject["hooks"].([]any)
	if len(hooks) == 0 {
		return ""
	}
	hook, _ := hooks[0].(map[string]any)
	command, _ := hook["command"].(string)
	return command
}

// mergeHookEntries adds to the hooks of `current` every matcher group of
// `desired` that the file does not already run, replaces a group of the module
// that runs the same command under another matcher, and keeps every other setting
// and hook. The layout is the one Claude Code defined and Codex shares.
func mergeHookEntries(current, desired []byte) ([]byte, error) {
	settings, err := decodeObject(current)
	if err != nil {
		return nil, err
	}
	wanted, err := decodeObject(desired)
	if err != nil {
		return nil, err
	}
	changed := false
	wantedEvents, _ := wanted[keyHooks].(map[string]any)
	for event, groups := range wantedEvents {
		list, _ := groups.([]any)
		for _, group := range list {
			command := firstCommand(group)
			if command == "" {
				continue
			}
			events := objectMember(settings, keyHooks)
			existing, _ := events[event].([]any)
			merged, updated := mergeGroup(existing, group, command)
			if updated {
				events[event] = merged
				changed = true
			}
		}
	}
	if !changed {
		return current, nil
	}
	return encodeObject(settings)
}

// mergeGroup puts a wanted matcher group in the groups of an event: appended when
// no group runs its command, swapped for the group of the same command that
// differs from it, and left out when the event already runs the command in a
// group the module does not own.
func mergeGroup(existing []any, group any, command string) ([]any, bool) {
	for index, held := range existing {
		if firstCommand(held) == command {
			if reflect.DeepEqual(held, group) {
				return existing, false
			}
			replaced := append([]any(nil), existing...)
			replaced[index] = group
			return replaced, true
		}
	}
	if mentions(existing, command) {
		return existing, false
	}
	return append(existing, group), true
}

// enablePlugin sets `enabledPlugins[name]` to true when the file does not mention
// the plugin, leaving a choice the user made, and every other setting, alone.
func enablePlugin(name string) func([]byte) ([]byte, error) {
	return func(current []byte) ([]byte, error) {
		settings, err := decodeObject(current)
		if err != nil {
			return nil, err
		}
		plugins := objectMember(settings, keyEnabledPlugins)
		if _, decided := plugins[name]; decided {
			return current, nil
		}
		plugins[name] = true
		return encodeObject(settings)
	}
}
